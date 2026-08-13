package proxy

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractForwardedHeaders_SafeListOnly(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("anthropic-version", "2023-06-01")
	r.Header.Set("OpenAI-Organization", "org-abc")
	r.Header.Set("X-Custom-Header", "should-not-leak")
	r.Header.Set("Authorization", "Bearer sk-should-not-leak")

	headers := extractForwardedHeaders(r.Header)

	require.NotNil(t, headers)
	assert.Equal(t, "2023-06-01", headers["anthropic-version"])
	assert.Equal(t, "org-abc", headers["OpenAI-Organization"])

	_, hasCustom := headers["X-Custom-Header"]
	assert.False(t, hasCustom, "X-Custom-Header should not be forwarded")
	_, hasAuth := headers["Authorization"]
	assert.False(t, hasAuth, "Authorization should not be forwarded")
}

func TestExtractForwardedHeaders_NilWhenNoneMatch(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("X-Only-Custom", "value")

	headers := extractForwardedHeaders(r.Header)
	assert.Nil(t, headers, "nil when no safe-listed headers present")
}

func TestExtractForwardedHeaders_PartialMatch(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("anthropic-beta", "beta-feature")
	// No other safe-listed headers.

	headers := extractForwardedHeaders(r.Header)

	require.NotNil(t, headers)
	assert.Len(t, headers, 1)
	assert.Equal(t, "beta-feature", headers["anthropic-beta"])
}
