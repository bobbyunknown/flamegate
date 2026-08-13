package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tetratelabs/wazero"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/domain/shared"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// InvokeEnv is captured by host function closures at InstantiateModule() time.
// Immutable after creation. Destroyed when instance is closed.
type InvokeEnv struct {
	Ctx          context.Context
	Slug         string
	Creds        core.Credentials
	ChunkCh      chan<- interface{}
	AllowedHosts []string
	AccountKey   string
	Logger       *logrus.Entry
	Vault        *vault.Vault
	AccountRepo  ports.AccountRepository
	HTTPClient   *http.Client
}

// WASMConnector implements core.Connector for a WASM extension.
type WASMConnector struct {
	engine   *Engine
	module   *Module
	slug     string
	compiled wazero.CompiledModule
	cfg      ExtensionConfig
}

// guestRequest is the JSON payload sent to the WASM guest's invoke function.
type guestRequest struct {
	Model    string            `json:"model"`
	Messages json.RawMessage   `json:"messages"`
	Stream   bool              `json:"stream"`
	Headers  map[string]string `json:"headers,omitempty"`
	Extra    map[string]any    `json:"extra,omitempty"`
}

// guestResponse is the JSON payload returned by the WASM guest's invoke function.
type guestResponse struct {
	Content      string `json:"content"`
	FinishReason string `json:"finish_reason,omitempty"`
	Error        string `json:"error,omitempty"`
	Code         string `json:"code,omitempty"`
}

// guestDelta carries incremental text in canonical OpenAI format.
type guestDelta struct {
	Content string `json:"content,omitempty"`
}

// guestChoice is one completion choice in canonical OpenAI chunk format.
type guestChoice struct {
	Delta guestDelta `json:"delta"`
	Index int        `json:"index"`
}

// guestUsage carries token accounting for done chunks.
type guestUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

// toStreamChunk converts a guest chunk to the canonical pipeline format.
func (g guestStreamChunk) toStreamChunk() shared.StreamChunk {
	switch g.Type {
	case "done":
		var usage *shared.Usage
		if g.Usage != nil {
			usage = &shared.Usage{
				PromptTokens:     g.Usage.PromptTokens,
				CompletionTokens: g.Usage.CompletionTokens,
				TotalTokens:      g.Usage.TotalTokens,
			}
		}
		return shared.StreamChunk{Type: shared.ChunkFinish, Usage: usage}
	case "error":
		return shared.StreamChunk{Type: shared.ChunkError, Err: fmt.Errorf("wasm: [%s] %s", g.Code, g.Error)}
	case "chunk":
		delta := ""
		if len(g.Choices) > 0 {
			delta = g.Choices[0].Delta.Content
		}
		return shared.StreamChunk{Type: shared.ChunkText, Delta: delta}
	default:
		return shared.StreamChunk{Type: shared.ChunkError, Err: fmt.Errorf("wasm: unknown chunk type %s", g.Type)}
	}
}

// guestStreamChunk is parsed from emit_chunk JSON payloads.
// Spec requires canonical OpenAI format: choices[].delta.content
type guestStreamChunk struct {
	Type     string        `json:"type"`
	Choices  []guestChoice `json:"choices,omitempty"`
	Usage    *guestUsage   `json:"usage,omitempty"`
	Model    string        `json:"model,omitempty"`
	Error    string        `json:"error,omitempty"`
	Code     string        `json:"code,omitempty"`
}

// emitGuestStreamChunk maps a guest chunk onto ch. Returns false when the
// stream should stop (done/error).
// Guests may omit "type" and only send OpenAI-shaped choices[].delta — treat
// that as a text chunk so stream content is not silently dropped.
func emitGuestStreamChunk(ch chan<- shared.StreamChunk, gChunk guestStreamChunk) bool {
	typ := gChunk.Type
	if typ == "" {
		switch {
		case gChunk.Error != "":
			typ = "error"
		case gChunk.Usage != nil && len(gChunk.Choices) == 0:
			typ = "done"
		case len(gChunk.Choices) > 0:
			typ = "chunk"
		default:
			return true
		}
	}
	switch typ {
	case "done":
		var usage *shared.Usage
		if gChunk.Usage != nil {
			usage = &shared.Usage{
				PromptTokens:     gChunk.Usage.PromptTokens,
				CompletionTokens: gChunk.Usage.CompletionTokens,
				TotalTokens:      gChunk.Usage.TotalTokens,
			}
		}
		ch <- shared.StreamChunk{Type: shared.ChunkFinish, Usage: usage}
		return false
	case "error":
		ch <- shared.StreamChunk{Type: shared.ChunkError, Err: fmt.Errorf("wasm: [%s] %s", gChunk.Code, gChunk.Error)}
		return false
	case "chunk":
		delta := ""
		if len(gChunk.Choices) > 0 {
			delta = gChunk.Choices[0].Delta.Content
		}
		if delta == "" {
			// Empty content deltas (role-only / finish frames) are not text.
			return true
		}
		ch <- shared.StreamChunk{
			Type:  shared.ChunkText,
			Delta: delta,
		}
		return true
	default:
		return true
	}
}


// decodeGuestChunk parses a payload from emit_chunk.
// hostEmitChunk sends raw JSON []byte; never json.Marshal those bytes
// (that base64-encodes them and drops every stream chunk).
func decodeGuestChunk(raw interface{}) (guestStreamChunk, bool) {
	var data []byte
	switch v := raw.(type) {
	case []byte:
		data = v
	case json.RawMessage:
		data = v
	case string:
		data = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return guestStreamChunk{}, false
		}
		data = b
	}
	var g guestStreamChunk
	if err := json.Unmarshal(data, &g); err != nil {
		return guestStreamChunk{}, false
	}
	return g, true
}


// ID returns the extension slug.
func (c *WASMConnector) ID() string { return c.slug }

// Dialect returns the canonical dialect (transform handles client dialect).
func (c *WASMConnector) Dialect() core.Dialect { return core.DialectOpenAI }

// Chat performs a non-streaming request via the WASM guest.
func (c *WASMConnector) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	start := time.Now()
	if c.engine.metrics != nil {
		c.engine.metrics.WASMInstancesActive.Inc()
		c.engine.metrics.WASMInstancesTotal.Inc()
		defer c.engine.metrics.WASMInstancesActive.Dec()
	}

	// Build guest request.
	msgs, err := json.Marshal(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("wasm: marshal messages: %w", err)
	}

	gReq := guestRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   false,
		Headers:  req.Headers,
	}
	if req.Extra != nil {
		gReq.Extra = make(map[string]any)
		for k, v := range req.Extra {
			gReq.Extra[k] = v
		}
	}

	// Acquire instance.
	log := logrus.WithField("slug", c.slug)
	env := &InvokeEnv{
		Ctx:          ctx,
		Slug:         c.slug,
		Creds:        creds,
		Logger:       log,
		Vault:        c.engine.vault,
		AccountRepo:  c.engine.accountRepo,
		AllowedHosts: c.cfg.AllowedHosts,
		HTTPClient:   c.engine.httpClient,
	}

	inst, err := c.module.Acquire(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("wasm: acquire %s: %w", c.slug, err)
	}
	defer c.module.Release(inst)

	// Serialize and write request to guest memory.
	reqPtr, reqSize, err := writeGuestJSON(ctx, inst, gReq)
	if err != nil {
		return nil, fmt.Errorf("wasm: write request: %w", err)
	}
	defer deallocGuest(ctx, inst, reqPtr, reqSize) //nolint:errcheck // best-effort dealloc

	// Call invoke.
	invokeFn := inst.ExportedFunction("invoke")
	if invokeFn == nil {
		return nil, fmt.Errorf("wasm: %s does not export invoke", c.slug)
	}

	results, err := invokeFn.Call(ctx, uint64(reqPtr), uint64(reqSize))
	if err != nil {
		return nil, fmt.Errorf("wasm: invoke %s: %w", c.slug, err)
	}

	respPtr := uint32(results[0])
	if respPtr == 0 {
		return nil, fmt.Errorf("wasm: invoke returned null pointer")
	}

	// Read response from guest memory.
	var gResp guestResponse
	if err := readGuestJSON(inst, respPtr, 0, &gResp); err != nil {
		// Try reading raw bytes and parsing manually.
		raw, readErr := readGuestBytes(inst, respPtr, 4096)
		if readErr != nil {
			return nil, fmt.Errorf("wasm: read response: %w", readErr)
		}
		if jsonErr := json.Unmarshal(raw, &gResp); jsonErr != nil {
			return nil, fmt.Errorf("wasm: parse response: %w", jsonErr)
		}
	}

	if gResp.Error != "" {
		return nil, fmt.Errorf("wasm: guest error [%s]: %s", gResp.Code, gResp.Error)
	}

	// Build canonical ChatResponse.
	resp := &shared.ChatResponse{
		ID:    fmt.Sprintf("wasm-%s-%d", c.slug, time.Now().UnixNano()),
		Model: req.Model,
		Message: shared.Message{
			Role: shared.RoleAssistant,
			Content: []shared.ContentPart{
				{Type: shared.PartText, Text: gResp.Content},
			},
		},
		FinishReason: shared.FinishReason(gResp.FinishReason),
	}

	if c.engine.metrics != nil {
		c.engine.metrics.WASMInvokeDuration.Observe(time.Since(start).Seconds())
	}
	return resp, nil
}

// Stream performs a streaming request via the WASM guest.
func (c *WASMConnector) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan shared.StreamChunk, error) {
	if c.engine.metrics != nil {
		c.engine.metrics.WASMInstancesActive.Inc()
		c.engine.metrics.WASMInstancesTotal.Inc()
	}
	// Build guest request with stream=true.
	msgs, err := json.Marshal(req.Messages)
	if err != nil {
		return nil, fmt.Errorf("wasm: marshal messages: %w", err)
	}

	gReq := guestRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   true,
		Headers:  req.Headers,
	}
	if req.Extra != nil {
		gReq.Extra = make(map[string]any)
		for k, v := range req.Extra {
			gReq.Extra[k] = v
		}
	}

	ch := make(chan shared.StreamChunk, 64)
	chunkCh := make(chan interface{}, 64)

	log := logrus.WithField("slug", c.slug)
	env := &InvokeEnv{
		Ctx:          ctx,
		Slug:         c.slug,
		Creds:        creds,
		ChunkCh:      chunkCh,
		Logger:       log,
		Vault:        c.engine.vault,
		AccountRepo:  c.engine.accountRepo,
		AllowedHosts: c.cfg.AllowedHosts,
		HTTPClient:   c.engine.httpClient,
	}

	// Acquire instance.
	inst, err := c.module.Acquire(ctx, env)
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("wasm: acquire %s: %w", c.slug, err)
	}

	// Serialize request to guest memory.
	reqPtr, reqSize, err := writeGuestJSON(ctx, inst, gReq)
	if err != nil {
		c.module.Release(inst)
		return nil, fmt.Errorf("wasm: write request: %w", err)
	}

	// Spawn goroutine to drive the stream.
	go func() {
		defer close(ch)
		defer c.module.Release(inst)
		defer deallocGuest(ctx, inst, reqPtr, reqSize) //nolint:errcheck // best-effort dealloc
		if c.engine.metrics != nil {
			defer c.engine.metrics.WASMInstancesActive.Dec()
			defer func() {
				if r := recover(); r != nil {
					c.engine.metrics.WASMPanicsTotal.WithLabelValues(c.slug).Inc()
					ch <- shared.StreamChunk{Type: shared.ChunkError, Err: fmt.Errorf("wasm: panic in %s: %v", c.slug, r)}
				}
			}()
		}

		// Start invoke in a separate goroutine (it blocks until guest returns).
		invokeDone := make(chan error, 1)
		go func() {
			invokeFn := inst.ExportedFunction("invoke")
			if invokeFn == nil {
				invokeDone <- fmt.Errorf("wasm: %s does not export invoke", c.slug)
				return
			}
			results, callErr := invokeFn.Call(ctx, uint64(reqPtr), uint64(reqSize))
			if callErr != nil {
				invokeDone <- callErr
				return
			}
			respPtr := uint32(results[0])
			if respPtr > 0 {
				// Read and check for error response.
				raw, readErr := readGuestBytes(inst, respPtr, 4096)
				if readErr == nil {
					var gResp guestResponse
					if jsonErr := json.Unmarshal(raw, &gResp); jsonErr == nil && gResp.Error != "" {
						invokeDone <- fmt.Errorf("wasm: guest error [%s]: %s", gResp.Code, gResp.Error)
						return
					}
				}
			}
			invokeDone <- nil
		}()

		// Stream chunks from channel to output.
		timeout := 30 * time.Second
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				// Timeout: no done chunk received.
				ch <- shared.StreamChunk{Type: shared.ChunkError, Err: fmt.Errorf("wasm: stream timeout after %s", timeout)}
				return
			case raw, ok := <-chunkCh:
				if !ok {
					return
				}
				timer.Reset(timeout)
				gChunk, ok := decodeGuestChunk(raw)
				if !ok {
					continue
				}
				if !emitGuestStreamChunk(ch, gChunk) {
					return
				}
			case err := <-invokeDone:
				// Drain any chunks already queued so done/text is not lost
				// when invoke returns before the host loop reads them.
				for {
					select {
					case raw, ok := <-chunkCh:
						if !ok {
							if err != nil {
								ch <- shared.StreamChunk{Type: shared.ChunkError, Err: err}
							}
							return
						}
						if gChunk, ok := decodeGuestChunk(raw); ok {
							if !emitGuestStreamChunk(ch, gChunk) {
								return
							}
						}
					default:
						if err != nil {
							ch <- shared.StreamChunk{Type: shared.ChunkError, Err: err}
						}
						return
					}
				}
			}
		}
	}()

	return ch, nil
}

// buildHostModule creates a wazero host module with the 4 host functions
// captured from env. Called per-instantiation.
func (e *Engine) buildHostModule(name string, env *InvokeEnv) wazero.HostModuleBuilder {
	hostBuilder := e.runtime.NewHostModuleBuilder(name)

	hostCfg := &HostFuncConfig{
		Slug:         env.Slug,
		Logger:       env.Logger,
		Creds:        env.Creds,
		Vault:        env.Vault,
		AccountRepo:  env.AccountRepo,
		AllowedHosts: env.AllowedHosts,
		HTTPClient:   env.HTTPClient,
	}

	// http_post
	hostBuilder.NewFunctionBuilder().
		WithFunc(hostHTTPPost(hostCfg.HTTPClient)).
		WithParameterNames("url_ptr", "url_len", "body_ptr", "body_len", "hdrs_ptr", "hdrs_len").
		Export("http_post")

	// http_get — list_models / OpenAI-compatible GET /models (spec non-stream discovery)
	hostBuilder.NewFunctionBuilder().
		WithFunc(hostHTTPGet(hostCfg.HTTPClient)).
		WithParameterNames("url_ptr", "url_len", "hdrs_ptr", "hdrs_len").
		Export("http_get")

	// get_credentials
	hostBuilder.NewFunctionBuilder().
		WithFunc(hostGetCredentials(hostCfg)).
		WithParameterNames("key_ptr", "key_len").
		Export("get_credentials")

	// emit_chunk
	hostBuilder.NewFunctionBuilder().
		WithFunc(hostEmitChunk(env.ChunkCh)).
		WithParameterNames("chunk_ptr", "chunk_len").
		Export("emit_chunk")

	// fg_log
	hostBuilder.NewFunctionBuilder().
		WithFunc(hostLog(env.Slug)).
		WithParameterNames("level_ptr", "level_len", "msg_ptr", "msg_len").
		Export("fg_log")

	return hostBuilder
}
