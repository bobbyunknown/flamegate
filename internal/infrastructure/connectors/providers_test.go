package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------



// openaiStreamChunk returns one OpenAI SSE data line.
func openaiStreamChunk(delta string) string {
	return fmt.Sprintf(`data: {"choices":[{"delta":{"role":"assistant","content":%q}}]}`, delta)
}

// ---------------------------------------------------------------------------
// Kiro Provider Tests (AWS EventStream binary protocol)
// ---------------------------------------------------------------------------



// ---------------------------------------------------------------------------
// Xiaomi MiMo Provider Tests (OpenAI-compatible protocol)
// ---------------------------------------------------------------------------

func TestXiaomiMiMo_Chat(t *testing.T) {
	// xiaomi-mimo requires streaming; the Chat method drains the stream
	// internally, so the mock must return SSE format.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, true, body["stream"], "xiaomi-mimo must send stream:true")

		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
	fmt.Fprintf(w, "%s\n\n", openaiStreamChunk("Hello from MiMo V2.5 Pro"))
	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n")
	fmt.Fprintf(w, "data: [DONE]\n\n")
		if flush != nil {
			flush.Flush()
		}
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	resp, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "test-api-key"})
	require.NoError(t, err)
	require.Equal(t, "Hello from MiMo V2.5 Pro", resp.Message.TextContent())
	require.Equal(t, core.FinishStop, resp.FinishReason)
	require.Equal(t, 5, resp.Usage.TotalTokens)
}

func TestXiaomiMiMo_Chat_MultipleModels(t *testing.T) {
	models := []struct {
		id      string
		display string
	}{
		{"mimo-v2.5-pro", "MiMo V2.5 Pro"},
		{"mimo-v2.5", "MiMo V2.5"},
		{"mimo-v2-omni", "MiMo V2 Omni"},
		{"mimo-v2-flash", "MiMo V2 Flash"},
	}

	for _, model := range models {
		t.Run(model.id, func(t *testing.T) {
			// xiaomi-mimo requires streaming; the Chat method drains the stream
			// internally, so the mock must return SSE format.
			expectedText := "ok from " + model.display
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				require.Equal(t, model.id, body["model"])
				require.Equal(t, true, body["stream"], "xiaomi-mimo must send stream:true")

				w.Header().Set("Content-Type", "text/event-stream")
				flush, _ := w.(http.Flusher)
		fmt.Fprintf(w, "%s\n\n", openaiStreamChunk(expectedText))
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
				if flush != nil {
					flush.Flush()
				}
			}))
			defer srv.Close()

			c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
			req := &core.ChatRequest{
				Model:  model.id,
				Stream: false,
				Messages: []core.Message{
					{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
				},
			}
			resp, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "test-key"})
			require.NoError(t, err)
			require.Equal(t, expectedText, resp.Message.TextContent())
		})
	}
}

func TestXiaomiMiMo_Stream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)

		w.Header().Set("Content-Type", "text/event-stream")
		flush, _ := w.(http.Flusher)
		lines := []string{
			openaiStreamChunk("He"),
			openaiStreamChunk("llo"),
			`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}
		for _, l := range lines {
		fmt.Fprintf(w, "%s\n\n", l)
			if flush != nil {
				flush.Flush()
			}
		}
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: true,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	ch, err := c.Stream(context.Background(), req, core.Credentials{APIKey: "test-api-key"}, core.StreamConfig{})
	require.NoError(t, err)

	var text string
	var finished bool
	for chunk := range ch {
		switch chunk.Type {
		case core.ChunkText:
			text += chunk.Delta
		case core.ChunkFinish:
			finished = true
		case core.ChunkError:
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
	}
	require.Equal(t, "Hello", text)
	require.True(t, finished)
}

func TestXiaomiMiMo_Chat_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprint(w, `{"error":{"message":"Invalid API key","type":"invalid_request_error"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "bad-key"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrAuth, pe.Kind)
	require.Equal(t, "xiaomi-mimo", pe.Provider)
	require.True(t, pe.Fallbackable())
}

func TestXiaomiMiMo_Chat_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprint(w, `{"error":{"message":"Rate limit exceeded"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "mimo-v2.5-pro",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "test-key"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrRateLimit, pe.Kind)
	require.Equal(t, 429, pe.StatusCode)
	require.True(t, pe.Fallbackable())
}

func TestXiaomiMiMo_Chat_BadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	fmt.Fprint(w, `{"error":{"message":"Model not found","type":"invalid_request_error"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	req := &core.ChatRequest{
		Model:  "nonexistent-model",
		Stream: false,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	_, err := c.Chat(context.Background(), req, core.Credentials{APIKey: "test-key"})
	require.Error(t, err)

	pe := core.AsProviderError(err)
	require.Equal(t, core.ErrBadRequest, pe.Kind)
	require.False(t, pe.Fallbackable(), "4xx request errors must not trigger fallback")
}

func TestXiaomiMiMo_Validate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/models", r.URL.Path)
		require.Equal(t, "GET", r.Method)
		w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"data":[{"id":"mimo-v2.5-pro","object":"model"}]}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	err := c.Validate(context.Background(), core.Credentials{APIKey: "test-key"})
	require.NoError(t, err)
}

func TestXiaomiMiMo_Validate_BadKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprint(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer srv.Close()

	c := NewOpenAICompatible("xiaomi-mimo", srv.URL)
	err := c.Validate(context.Background(), core.Credentials{APIKey: "bad-key"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation failed")
}

func TestXiaomiMiMo_Dialect(t *testing.T) {
	c := NewOpenAICompatible("xiaomi-mimo", "http://unused")
	require.Equal(t, core.DialectOpenAI, c.Dialect())
}

// ---------------------------------------------------------------------------
// Xiaomi Token Plan Provider Tests
// ---------------------------------------------------------------------------

