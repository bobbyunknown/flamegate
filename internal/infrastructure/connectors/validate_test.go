package connectors

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)


// modelsServer returns a test server that responds to GET requests with the
// given status. A 200 returns a minimal OpenAI-style models list.
func modelsServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if status >= 400 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`)) //nolint:errcheck // best-effort write
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"m1"}]}`)) //nolint:errcheck // best-effort write
	}))
}

func TestGemini_Validate(t *testing.T) {
	t.Run("valid key", func(t *testing.T) {
		srv := modelsServer(t, http.StatusOK)
		defer srv.Close() //nolint:errcheck // best-effort close
		c := NewGemini("gemini", srv.URL)
		require.NoError(t, c.Validate(context.Background(), core.Credentials{APIKey: "k", BaseURL: srv.URL}))
	})
	t.Run("rejected key", func(t *testing.T) {
		srv := modelsServer(t, http.StatusUnauthorized)
		defer srv.Close() //nolint:errcheck // best-effort close
		c := NewGemini("gemini", srv.URL)
		require.Error(t, c.Validate(context.Background(), core.Credentials{APIKey: "bad", BaseURL: srv.URL}))
	})
	t.Run("missing credential", func(t *testing.T) {
		c := NewGemini("gemini", "https://example.com")
		require.Error(t, c.Validate(context.Background(), core.Credentials{}))
	})
}


