package connectors

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/transform"
)

// Gemini drives Google's Generative Language API (generateContent /
// streamGenerateContent). Unlike OpenAI/Anthropic, Gemini embeds the model and
// the action in the URL path ("/models/{model}:generateContent") rather than
// the request body, and authenticates with an API key via the x-goog-api-key
// header (or an OAuth bearer token for Cloud-authenticated callers).
type Gemini struct {
	id          string
	defaultBase string
	codec       transform.GeminiCodec
}

// NewGemini builds a Gemini connector for the given provider id and default
// base URL (e.g. https://generativelanguage.googleapis.com/v1beta).
func NewGemini(id, defaultBaseURL string) *Gemini {
	return &Gemini{id: id, defaultBase: defaultBaseURL}
}

func (c *Gemini) ID() string            { return c.id }
func (c *Gemini) Dialect() core.Dialect { return core.DialectGemini }

func (c *Gemini) baseURL(creds core.Credentials) string {
	if creds.BaseURL != "" {
		return creds.BaseURL
	}
	return c.defaultBase
}

// headers builds the upstream auth headers. Gemini accepts the API key either
// as a ?key= query param or the x-goog-api-key header; we use the header to
// keep the key out of URLs/logs. OAuth callers send a bearer token instead.
func (c *Gemini) headers(creds core.Credentials) map[string]string {
	h := map[string]string{}
	switch {
	case creds.AccessToken != "":
		h["Authorization"] = bearer(creds.AccessToken)
	case creds.APIKey != "":
		h["x-goog-api-key"] = creds.APIKey
	}
	return mergeHeaders(h, creds.Headers)
}

// modelURL builds the generateContent / streamGenerateContent endpoint for a
// given model. The model is percent-escaped to survive any unusual ids.
func (c *Gemini) modelURL(creds core.Credentials, model, action string) string {
	base := c.baseURL(creds)
	return joinURL(base, "models/"+url.PathEscape(model)+":"+action)
}

// Validate probes the upstream by listing models (GET /models), confirming the
// API key or OAuth token is accepted. A 401/403 means the credential is bad.
func (c *Gemini) Validate(ctx context.Context, creds core.Credentials) error {
	if creds.APIKey == "" && creds.AccessToken == "" {
		return fmt.Errorf("validation failed for %s: no API key or access token", c.id)
	}
	base := strings.TrimRight(c.baseURL(creds), "/")
	if _, err := doJSONMethod(ctx, http.MethodGet, c.id, "validate", base+"/models", nil, c.headers(creds)); err != nil {
		return fmt.Errorf("validation failed for %s: %w", c.id, err)
	}
	return nil
}

// Chat performs a non-streaming generateContent call.
func (c *Gemini) Chat(ctx context.Context, req *core.ChatRequest, creds core.Credentials) (*core.ChatResponse, error) {
	req.Stream = false
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	url := c.modelURL(creds, req.Model, "generateContent")
	respBody, err := doJSON(ctx, c.id, req.Model, url, body, c.headers(creds))
	if err != nil {
		return nil, err
	}

	resp, err := c.codec.ParseResponse(respBody, req.Model)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrUpstream, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}
	return resp, nil
}

// StreamRaw opens a streaming SSE connection and returns the raw response body
// for zero-copy same-dialect piping.
func (c *Gemini) StreamRaw(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (io.ReadCloser, http.Header, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	streamURL := c.modelURL(creds, req.Model, "streamGenerateContent") + "?alt=sse"
	resp, err := openStream(ctx, c.id, req.Model, streamURL, body, c.headers(creds))
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp.Header, nil
}

// Stream performs a streaming streamGenerateContent call. Gemini emits SSE when
// asked with ?alt=sse; each data line is a partial generateContent response
// that the codec maps to canonical chunks.
func (c *Gemini) Stream(ctx context.Context, req *core.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan core.StreamChunk, error) {
	req.Stream = true
	body, err := c.codec.RenderRequest(req)
	if err != nil {
		return nil, &core.ProviderError{Kind: core.ErrInternal, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err}
	}

	streamURL := c.modelURL(creds, req.Model, "streamGenerateContent") + "?alt=sse"
	resp, err := openStream(ctx, c.id, req.Model, streamURL, body, c.headers(creds))
	if err != nil {
		return nil, err
	}

	out := make(chan core.StreamChunk, 16)
	go func() {
		defer close(out)
		defer resp.Body.Close() //nolint:errcheck // best-effort close

		ttft := newTTFTTracker(cfg)

		scanner := sseScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			payload, ok := parseSSEData(scanner.Text())
			if !ok {
				continue
			}
			chunks, perr := c.codec.ParseStreamLine([]byte(payload), req.Model)
			if perr != nil {
				continue
			}
			for _, ch := range chunks {
				ttft.maybeReport(ch)
				select {
				case out <- ch:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			out <- core.StreamChunk{
				Type: core.ChunkError,
				Err:  &core.ProviderError{Kind: core.ErrTimeout, Provider: c.id, Model: req.Model, Message: err.Error(), Cause: err},
			}
		}
	}()
	return out, nil
}
