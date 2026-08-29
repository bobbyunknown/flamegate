package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/domain/shared"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/dispatch"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/pipeline"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/transform"
	"github.com/bobbyunknown/flamegate/internal/shared/limits"
)

func (s *Handler) HandleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	s.HandleChat(w, r, core.DialectOpenAI)
}

// handleAnthropicMessages serves /v1/messages in the Anthropic dialect.
func (s *Handler) HandleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	s.HandleChat(w, r, core.DialectAnthropic)
}

// handleAnthropicCountTokens serves /v1/messages/count_tokens. Anthropic
// clients (notably Claude Code) call this before each /v1/messages turn to size
// the context window. We do not forward it upstream — most OpenAI-dialect
// providers (e.g. Xiaomi MiMo) have no equivalent endpoint and would return 405
// — so we parse the request locally and return a heuristic estimate in the
// Anthropic response shape: {"input_tokens": N}. The estimate uses the common
// ~4 chars/token rule, which is accurate enough for client-side budgeting.
func (s *Handler) HandleAnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	codec, err := s.codecs.Codec(core.DialectAnthropic)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "unsupported dialect")
		return
	}
	req, err := codec.ParseRequest(body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	resp := struct {
		InputTokens int `json:"input_tokens"`
	}{InputTokens: estimateInputTokens(req)}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// estimateInputTokens approximates the prompt token count for a request using
// the ~4 chars/token heuristic over system text, message content, tool-call
// arguments, and tool results.
func estimateInputTokens(req *core.ChatRequest) int {
	return core.EstimatePromptTokens(req)
}

// handleOpenAIResponses serves /v1/responses in the OpenAI Responses dialect
// (Codex and Responses-native clients).
func (s *Handler) HandleOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	s.HandleChat(w, r, core.DialectOpenAIResponses)
}

// handleChat is the shared chat handler parameterized by the client dialect.
func (s *Handler) HandleChat(w http.ResponseWriter, r *http.Request, dialect core.Dialect) {
	key, _ := authedKey(r.Context())
	tenantID := tenantOf(key)
	client := detectClient(r)

	s.consoleLog.Log("DEBUG",
		fmt.Sprintf("New request from %q (%s API)", client, dialect),
		fmt.Sprintf("Method: %s\nPath:   %s\nClient: %s\nDialect: %s\nKey:    %s (%s)",
			r.Method, r.URL.Path, client, dialect, key.Name, key.ID))

	codec, err := s.codecs.Codec(dialect)
	if err != nil {
		s.consoleLog.Log("ERROR", fmt.Sprintf("Unsupported API dialect: %s", dialect), "")
		WriteError(w, http.StatusInternalServerError, "unsupported dialect")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		s.consoleLog.Log("ERROR", "Failed to read request body", err.Error())
		WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	s.consoleLog.Log("DEBUG", fmt.Sprintf("Read request body (%s)", humanBytes(len(body))), "")

	req, err := codec.ParseRequest(body)
	if err != nil {
		s.consoleLog.Log("ERROR", "Failed to parse request body", err.Error())
		WriteError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}

	// Attach routing metadata.
	req.Metadata = core.RequestMetadata{
		ClientKind:    client,
		SourceDialect: dialect,
		APIKeyID:      key.ID,
		TenantID:      tenantID,
		ProjectID:     key.ProjectID,
		RequestID:     chimiddleware.GetReqID(r.Context()),
	}

	req.Headers = extractForwardedHeaders(r.Header)

	streamNote := ""
	if req.Stream {
		streamNote = " · streaming"
	}
	s.consoleLog.Log("DEBUG",
		fmt.Sprintf("Routing %q · %d message%s%s", req.Model, len(req.Messages), plural(len(req.Messages)), streamNote),
		fmt.Sprintf("Model:    %s\nMessages: %d\nStream:   %v\nTenant:   %s\nKey:      %s (%s)",
			req.Model, len(req.Messages), req.Stream, tenantID, key.Name, key.ID))

	resolved, err := resolveTargets(r.Context(), s.chains, s.aliases, s.latencyReader(), tenantID, req.Model)
	if err != nil {
		var bad badModelError
		if errors.As(err, &bad) {
			s.consoleLog.Log("WARN", fmt.Sprintf("Unknown model %q", req.Model), bad.Error())
			WriteError(w, http.StatusBadRequest, bad.Error())
			return
		}
		s.consoleLog.Log("ERROR", fmt.Sprintf("Failed to resolve model %q", req.Model), err.Error())
		WriteError(w, http.StatusInternalServerError, "failed to resolve model")
		return
	}

	// Surface the first resolved provider into the routing metadata so the
	// guardrails resolver can apply provider-scoped policies. The first
	// target is the primary; fallback targets may differ but policy lookups
	// happen once per request, before dispatch.
	if len(resolved.Targets) > 0 {
		req.Metadata.Provider = resolved.Targets[0].Provider
	}
	req.Metadata.ChainID = resolved.PlanOpts.ChainID

	// Enforce per-key model access restrictions. Filter resolved targets to
	// only include models the key is allowed to access.
	if len(resolved.Targets) > 0 {
		filtered, ferr := s.filterAllowedTargets(r.Context(), key.ID, resolved.Targets)
		if ferr != nil {
			s.consoleLog.Log("ERROR", "Model access check failed", ferr.Error())
			WriteError(w, http.StatusInternalServerError, "model access check failed")
			return
		}
		if len(filtered) == 0 {
			s.consoleLog.Log("WARN",
				fmt.Sprintf("Access denied · key %q may not use %q", key.Name, req.Model),
				fmt.Sprintf("Key:   %s (%s)\nModel: %s", key.Name, key.ID, req.Model))
			WriteError(w, http.StatusForbidden, "access denied: this API key is not permitted to use model "+req.Model)
			return
		}
		resolved.Targets = filtered
	}

	if len(resolved.Targets) > 0 {
		primary := resolved.Targets[0]
		var tb strings.Builder
		for i, t := range resolved.Targets {
			if i > 0 {
				tb.WriteByte('\n')
			}
			fmt.Fprintf(&tb, "%d. %s/%s", i+1, t.Provider, t.Model)
		}
		msg := fmt.Sprintf("Resolved to %s/%s", primary.Provider, primary.Model)
		if len(resolved.Targets) > 1 {
			msg = fmt.Sprintf("%s (+%d fallback%s)", msg, len(resolved.Targets)-1, plural(len(resolved.Targets)-1))
		}
		s.consoleLog.Log("DEBUG", msg, tb.String())
	}
	affinityKey := requestAffinityKey(r, req)
	req.Metadata.ContextAffinityKey = affinityKey

	effectiveLimits, err := s.effectiveLimits(r.Context(), key)
	if err != nil {
		s.consoleLog.Log("ERROR", "Failed to resolve rate limits", err.Error())
		WriteError(w, http.StatusInternalServerError, "limit resolution failed")
		return
	}

	opts := pipeline.Options{
		Targets:  resolved.Targets,
		PlanOpts: s.endpointPlanOptions(r.Context(), resolved.PlanOpts, resolved.Targets, affinityKey),
		Slimmer:  s.slimmerConfig(),
		Terse:    s.terseConfig(),
		Caveman:  s.cavemanConfig(),
		Headroom: s.headroomConfig(),
		Ponytail: s.ponytailConfig(),
		Limits:   effectiveLimits,
	}

	if req.Stream {
		s.consoleLog.Log("DEBUG", "Dispatching as streaming response", "")
		s.streamChat(w, r, codec, req, opts, key.Name)
		return
	}
	s.consoleLog.Log("DEBUG", "Dispatching as standard response", "")
	s.unaryChat(w, r, codec, req, opts, key.Name)
}

func (s *Handler) effectiveLimits(ctx context.Context, key schema.APIKey) (limits.EffectiveLimits, error) {
	if key.PlanID != "" {
		plan, err := s.db.Plans().Get(ctx, key.PlanID)
		if err != nil {
			return limits.EffectiveLimits{}, err
		}
		return limits.EffectiveLimits{
			RPM:         plan.RPMLimit,
			TPM:         plan.TPMLimit,
			Concurrency: plan.ConcurrencyLimit,
		}, nil
	}
	return limits.EffectiveLimits{
		RPM:         s.cfg.Limits.DefaultRPM,
		TPM:         s.cfg.Limits.DefaultTPM,
		Concurrency: s.cfg.Limits.DefaultConcurrency,
	}, nil
}

// unaryChat runs a non-streaming request and renders the response.
func (s *Handler) unaryChat(w http.ResponseWriter, r *http.Request, codec transform.Codec, req *core.ChatRequest, opts pipeline.Options, keyName string) {
	start := time.Now()
	s.consoleLog.Log("DEBUG", "Sending request to provider…", "")
	result, err := s.pipeline.Chat(r.Context(), req, opts)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		s.consoleLog.Log("ERROR", fmt.Sprintf("Provider request failed after %s", humanDuration(latency)), err.Error())
		s.logRequest(keyName, req.Model, req.Model, 0, 0, latency, false, err)
		s.writeProviderError(w, err)
		return
	}

	out, err := codec.RenderResponse(result.Response)
	if err != nil {
		s.consoleLog.Log("ERROR", "Failed to render provider response", err.Error())
		WriteError(w, http.StatusInternalServerError, "failed to render response")
		return
	}
	tokens := result.Response.Usage.PromptTokens + result.Response.Usage.CompletionTokens
	s.consoleLog.Log("DEBUG",
		fmt.Sprintf("Response from %s/%s · %s tokens · %s", result.Provider, result.Model, humanInt(tokens), humanDuration(latency)),
		fmt.Sprintf("Provider: %s\nModel:    %s\nTokens:   %s\nAccount:  %s\nCache:    %v\nLatency:  %dms",
			result.Provider, result.Model, humanInt(tokens), result.AccountID, result.CacheHit, latency))
	s.logRequest(keyName, result.Provider, result.Model, tokens, result.CostMicros, latency, result.CacheHit, nil)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-FlameGate-Provider", result.Provider)
	w.Header().Set("X-FlameGate-Model", result.Model)
	if result.CacheHit {
		w.Header().Set("X-FlameGate-Cache", "hit")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// streamChat runs a streaming request and relays SSE events in the client's
// dialect, honoring client disconnects and the configured stall timeout.
func (s *Handler) streamChat(w http.ResponseWriter, r *http.Request, codec transform.Codec, req *core.ChatRequest, opts pipeline.Options, keyName string) {
	streamCodec, ok := codec.(transform.StreamCodec)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "dialect does not support streaming")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming unsupported by server")
		return
	}

	start := time.Now()
	s.consoleLog.Log("DEBUG", "Opening stream to provider…", "")
	result, err := s.pipeline.Stream(r.Context(), req, opts)
	if err != nil {
		latency := int(time.Since(start).Milliseconds())
		s.consoleLog.Log("ERROR", fmt.Sprintf("Stream failed to start after %s", humanDuration(latency)), err.Error())
		s.logRequest(keyName, req.Model, req.Model, 0, 0, latency, false, err)
		s.writeProviderError(w, err)
		return
	}
	s.consoleLog.Log("DEBUG",
		fmt.Sprintf("Streaming from %s/%s", result.Provider, result.Model),
		fmt.Sprintf("Provider: %s\nModel:    %s\nAccount:  %s", result.Provider, result.Model, result.AccountID))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-FlameGate-Provider", result.Provider)
	w.Header().Set("X-FlameGate-Model", result.Model)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Zero-copy direct pipe path: the pipeline detected same-dialect, no-tools
	// and obtained a raw io.ReadCloser from the upstream. Pipe it directly to
	// the client via io.Copy — no JSON parse/serialize, no goroutines, minimal
	// memory allocation. This is the fastest possible streaming path.
	if result.DirectBody != nil {
		defer result.DirectBody.Close()
		n, cpErr := io.Copy(w, result.DirectBody)
		if cpErr != nil && !isClientDisconnect(cpErr) {
			s.consoleLog.Log("ERROR", fmt.Sprintf("Stream interrupted after %s", humanBytes(int(n))), cpErr.Error())
			s.log.Warn("direct pipe error", "bytes", n, "err", cpErr)
		}
		flusher.Flush()
		// Record usage from the captured SSE stream. The pipeline wraps
		// the direct body in a tee reader that captures all bytes; the
		// DirectUsageFunc parses the captured data for usage tokens.
		if result.DirectUsageFunc != nil {
			result.DirectUsageFunc()
		}
		latency := int(time.Since(start).Milliseconds())
		s.consoleLog.Log("DEBUG", fmt.Sprintf("Stream finished · %s · %s", humanBytes(int(n)), humanDuration(latency)), "")
		s.logRequest(keyName, result.Provider, result.Model, 0, 0, latency, false, nil)
		return
	}

	// Wrap the response writer in a bufio.Writer to batch small SSE writes
	// into fewer syscalls. The pool avoids allocating a new writer per request.
	bw := shared.SSEWriterPool.Get().(*bufio.Writer)
	defer shared.SSEWriterPool.Put(bw)
	bw.Reset(w)

	state := &transform.StreamState{Model: result.Model}
	streamStart := time.Now()
	var totalTokens int
	var chunkCount int

	// ToolArgSanitizer buffers streaming tool call arguments and emits
	// sanitized JSON when each tool call completes. This fixes malformed
	// arguments from non-Anthropic models (e.g., Read.limit as string).
	// Tool-call args from fragmenting upstreams (Kiro, Cursor, CommandCode)
	// arrive split across frames and must be reassembled into one complete JSON
	// object before rendering, regardless of tool name or client dialect.
	// Streaming raw fragments and relying on the client to reassemble breaks
	// clients like Cline ("missing required parameter"). The sanitizer passes
	// text/thinking through immediately, so this only buffers the (small,
	// non-actionable) tool-arg fragments — live text streaming is unaffected.
	sanitizer := transform.NewToolArgSanitizer()

	// ThinkTagState strips <think>...</think> tags from streaming content.

	// Some models (MiMo, QwQ) embed reasoning as XML tags in the content
	// field instead of using a structured reasoning_content field.
	thinkFilter := &transform.ThinkTagState{}
	renderChunk := func(cleaned core.StreamChunk) {
		// Route thinking chunks through the filter; tool calls and others
		// pass through directly.
		if cleaned.Type == core.ChunkText {
			for _, fc := range thinkFilter.ProcessFeed(cleaned.Delta) {
				if fc.Type == core.ChunkThinking {
					// Thinking content is consumed internally — not sent to client.
					continue
				}
				events, rerr := streamCodec.RenderStreamChunk(fc, state)
				if rerr != nil {
					s.log.Warn("failed to render stream chunk", "err", rerr)
					return
				}
				for _, ev := range events {
					if _, werr := bw.Write(ev); werr != nil {
						s.consoleLog.Log("WARN", fmt.Sprintf("Client disconnected after %d chunks", chunkCount), "")
						return
					}
				}
			}
			bw.Flush()
			flusher.Flush()
			return
		}
		events, rerr := streamCodec.RenderStreamChunk(cleaned, state)
		if rerr != nil {
			s.log.Warn("failed to render stream chunk", "err", rerr)
			return
		}
		for _, ev := range events {
			if _, werr := bw.Write(ev); werr != nil {
				s.consoleLog.Log("WARN", fmt.Sprintf("Client disconnected after %d chunks", chunkCount), "")
				return
			}
		}
		// Flush the buffered writer to the underlying http.ResponseWriter,
		// then flush the HTTP flusher to push bytes to the client.
		bw.Flush()
		flusher.Flush()
	}

	for chunk := range result.Chunks {
		if chunk.Type == core.ChunkError {
			s.consoleLog.Log("ERROR", "Provider stream error", fmt.Sprintf("%v", chunk.Err))
			s.log.Warn("stream error", "err", chunk.Err)
			break
		}
		if chunk.Type == core.ChunkUsage && chunk.Usage != nil {
			totalTokens = chunk.Usage.PromptTokens + chunk.Usage.CompletionTokens
		}
		chunkCount++
		sanitizer.Process(chunk, renderChunk)
	}

	// Flush any remaining buffered tool calls and think-tag buffer.
	sanitizer.Flush(renderChunk)

	// Flush think-tag state — emit any remaining buffered text.

	for _, fc := range thinkFilter.Flush() {
		if fc.Type == core.ChunkThinking {
			continue
		}
		events, _ := streamCodec.RenderStreamChunk(fc, state)
		for _, ev := range events {
			_, _ = bw.Write(ev)
		}
	}

	for _, ev := range streamCodec.RenderStreamDone(state) {
		_, _ = bw.Write(ev)
	}
	bw.Flush()
	flusher.Flush()

	latency := int(time.Since(streamStart).Milliseconds())
	s.consoleLog.Log("DEBUG",
		fmt.Sprintf("Stream complete · %d chunks · %s tokens · %s", chunkCount, humanInt(totalTokens), humanDuration(latency)),
		fmt.Sprintf("Provider: %s\nModel:    %s\nChunks:   %d\nTokens:   %s\nLatency:  %dms",
			result.Provider, result.Model, chunkCount, humanInt(totalTokens), latency))
	s.logRequest(keyName, result.Provider, result.Model, totalTokens, 0, latency, false, nil)
}

// writeProviderError maps a structured provider error to an HTTP status.
func (s *Handler) writeProviderError(w http.ResponseWriter, err error) {
	pe := core.AsProviderError(err)
	status := http.StatusBadGateway
	switch pe.Kind {
	case core.ErrBadRequest:
		status = http.StatusBadRequest
	case core.ErrAuth:
		status = http.StatusUnauthorized
	case core.ErrRateLimit:
		status = http.StatusTooManyRequests
		if pe.RetryAfter > 0 {
			w.Header().Set("Retry-After", fmt.Sprintf("%.0f", pe.RetryAfter.Seconds()))
		}
	case core.ErrQuotaExhausted, core.ErrBudgetBlocked:
		status = http.StatusPaymentRequired
	case core.ErrTimeout:
		status = http.StatusGatewayTimeout
	case core.ErrInternal:
		status = http.StatusInternalServerError
	}
	WriteError(w, status, pe.Message)
}

// isClientDisconnect reports whether an error is a client disconnection
// (broken pipe, reset by peer) rather than a server-side failure. These are
// expected during streaming and should not be logged as errors.
func isClientDisconnect(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "broken pipe") ||
		strings.Contains(s, "reset by peer") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "use of closed network connection")
}

// filterAllowedTargets filters resolved routing targets to only include models
// the given API key is allowed to access. Returns empty slice if no target
// matches the key's model access policy.
func (s *Handler) filterAllowedTargets(ctx context.Context, keyID string, targets []dispatch.Target) ([]dispatch.Target, error) {
	keys := s.identity.Keys()
	allowed, err := keys.GetAllowedModels(ctx, keyID)
	if err != nil {
		return nil, err
	}
	if len(allowed) == 0 {
		return targets, nil // no restriction
	}

	// Match all targets in-memory against the already-fetched allowed list.
	// This avoids N additional DB round-trips (one per target) that the
	// previous IsModelAllowed-per-target pattern caused.
	var filtered []dispatch.Target
	for _, t := range targets {
		if modelMatchesAny(t.Model, allowed) {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// modelMatchesAny reports whether model matches any pattern in allowed.
// Patterns support a trailing '*' wildcard (e.g. "claude-*").
func modelMatchesAny(model string, allowed []string) bool {
	lower := strings.ToLower(model)
	for _, pattern := range allowed {
		lp := strings.ToLower(pattern)
		if strings.HasSuffix(lp, "*") {
			if strings.HasPrefix(lower, lp[:len(lp)-1]) {
				return true
			}
		} else if lp == lower {
			return true
		}
	}
	return false
}

// handleKeyUsage serves GET /v1/keys/me/usage — the authenticated API key
