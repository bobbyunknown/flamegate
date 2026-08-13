package transform

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenAI_ParseRequest_IncludeUsage verifies stream_options.include_usage is
// captured into the canonical request so the pipeline can guarantee a usage
// event on the streaming response.
func TestOpenAI_ParseRequest_IncludeUsage(t *testing.T) {
	t.Run("opted in", func(t *testing.T) {
		body := []byte(`{"model":"gpt-4o","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`)
		req, err := OpenAICodec{}.ParseRequest(body)
		require.NoError(t, err)
		require.True(t, req.IncludeUsage)
	})

	t.Run("opted out", func(t *testing.T) {
		body := []byte(`{"model":"gpt-4o","stream":true,"stream_options":{"include_usage":false},"messages":[{"role":"user","content":"hi"}]}`)
		req, err := OpenAICodec{}.ParseRequest(body)
		require.NoError(t, err)
		require.False(t, req.IncludeUsage)
	})

	t.Run("no stream_options", func(t *testing.T) {
		body := []byte(`{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
		req, err := OpenAICodec{}.ParseRequest(body)
		require.NoError(t, err)
		require.False(t, req.IncludeUsage)
	})

	t.Run("non-stream request ignores include_usage", func(t *testing.T) {
		body := []byte(`{"model":"gpt-4o","stream":false,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hi"}]}`)
		req, err := OpenAICodec{}.ParseRequest(body)
		require.NoError(t, err)
		require.False(t, req.IncludeUsage)
	})
}
