package transform

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestGemini_ParseStreamLine_TextAndUsage(t *testing.T) {
	line := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hel"}]}}]}`)
	chunks, err := GeminiCodec{}.ParseStreamLine(line, "gemini-2.0-flash")
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Equal(t, core.ChunkText, chunks[0].Type)
	require.Equal(t, "hel", chunks[0].Delta)

	final := []byte(`{"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}`)
	chunks, err = GeminiCodec{}.ParseStreamLine(final, "gemini-2.0-flash")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(chunks), 2)
	require.Equal(t, core.ChunkFinish, chunks[0].Type)
	require.Equal(t, core.FinishStop, chunks[0].FinishReason)
	require.Equal(t, core.ChunkUsage, chunks[1].Type)
	require.Equal(t, 4, chunks[1].Usage.TotalTokens)
}

func TestGemini_ParseStreamLine_FunctionCall(t *testing.T) {
	line := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"SF"}}}]}}]}`)
	chunks, err := GeminiCodec{}.ParseStreamLine(line, "gemini-2.0-flash")
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.Equal(t, core.ChunkToolCall, chunks[0].Type)
	require.Equal(t, "get_weather", chunks[0].ToolCall.Name)
	require.JSONEq(t, `{"city":"SF"}`, string(chunks[0].ToolCall.Arguments))
}

func TestGemini_RenderStreamChunk_SSEShape(t *testing.T) {
	state := &StreamState{Model: "gemini-2.0-flash"}
	out, err := GeminiCodec{}.RenderStreamChunk(core.StreamChunk{Type: core.ChunkText, Delta: "hi"}, state)
	require.NoError(t, err)
	require.Len(t, out, 1)
	s := string(out[0])
	require.True(t, strings.HasPrefix(s, "data: "))
	require.True(t, strings.HasSuffix(s, "\n\n"))
	require.Contains(t, s, `"text":"hi"`)
	require.Contains(t, s, `"role":"model"`)

	usage, err := GeminiCodec{}.RenderStreamChunk(core.StreamChunk{
		Type: core.ChunkUsage, Usage: &core.Usage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4},
	}, state)
	require.NoError(t, err)
	require.Len(t, usage, 1)
	require.Contains(t, string(usage[0]), `"totalTokenCount":4`)

	// Gemini SSE has no terminal sentinel.
	require.Empty(t, GeminiCodec{}.RenderStreamDone(state))
}
