package wasm

import (
	"encoding/json"
	"testing"

	"github.com/bobbyunknown/flamegate/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuestStreamChunk_CanonicalFormat(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantContent  string
		wantModel    string
		wantType     string
		wantUsage    bool
		wantFinished bool
	}{
		{
			name:        "canonical chunk with content",
			input:       `{"type":"chunk","choices":[{"delta":{"content":"hello"},"index":0}],"model":"kiro-v1"}`,
			wantContent: "hello",
			wantModel:   "kiro-v1",
			wantType:    "chunk",
		},
		{
			name:        "canonical chunk with empty content",
			input:       `{"type":"chunk","choices":[{"delta":{},"index":0}]}`,
			wantContent: "",
			wantType:    "chunk",
		},
		{
			name:         "done chunk with usage",
			input:        `{"type":"done","usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`,
			wantType:     "done",
			wantUsage:    true,
			wantFinished: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var chunk guestStreamChunk
			err := json.Unmarshal([]byte(tt.input), &chunk)
			require.NoError(t, err)

			assert.Equal(t, tt.wantType, chunk.Type)

			if tt.wantContent != "" || len(chunk.Choices) > 0 {
				gotContent := ""
				if len(chunk.Choices) > 0 {
					gotContent = chunk.Choices[0].Delta.Content
				}
				assert.Equal(t, tt.wantContent, gotContent)
			}

			if tt.wantModel != "" {
				assert.Equal(t, tt.wantModel, chunk.Model)
			}

			if tt.wantUsage {
				require.NotNil(t, chunk.Usage)
				assert.Equal(t, 10, chunk.Usage.PromptTokens)
				assert.Equal(t, 20, chunk.Usage.CompletionTokens)
				assert.Equal(t, 30, chunk.Usage.TotalTokens)
			}
		})
	}
}

func TestConvertGuestChunkToStreamChunk(t *testing.T) {
	tests := []struct {
		name    string
		chunk   guestStreamChunk
		wantErr bool
		verify  func(t *testing.T, sc shared.StreamChunk)
	}{
		{
			name: "text chunk",
			chunk: guestStreamChunk{
				Type: "chunk",
				Choices: []guestChoice{
					{Delta: guestDelta{Content: "hello"}, Index: 0},
				},
				Model: "kiro-v1",
			},
			verify: func(t *testing.T, sc shared.StreamChunk) {
				assert.Equal(t, shared.ChunkText, sc.Type)
				assert.Equal(t, "hello", sc.Delta)
			},
		},
		{
			name: "done with usage",
			chunk: guestStreamChunk{
				Type: "done",
				Usage: &guestUsage{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
				},
			},
			verify: func(t *testing.T, sc shared.StreamChunk) {
				assert.Equal(t, shared.ChunkFinish, sc.Type)
				require.NotNil(t, sc.Usage)
				assert.Equal(t, 10, sc.Usage.PromptTokens)
				assert.Equal(t, 20, sc.Usage.CompletionTokens)
			},
		},
		{
			name: "error chunk",
			chunk: guestStreamChunk{
				Type:  "error",
				Code:  "rate_limit",
				Error: "too many requests",
			},
			verify: func(t *testing.T, sc shared.StreamChunk) {
				assert.Equal(t, shared.ChunkError, sc.Type)
				assert.Error(t, sc.Err)
				assert.Contains(t, sc.Err.Error(), "rate_limit")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := tt.chunk.toStreamChunk()
			tt.verify(t, sc)
		})
	}
}

func TestDecodeGuestChunk_RawBytesNotBase64(t *testing.T) {
	// hostEmitChunk sends []byte JSON. Marshaling that would base64-encode and break stream.
	raw := []byte(`{"type":"chunk","choices":[{"delta":{"content":"OK"},"index":0}]}`)
	g, ok := decodeGuestChunk(raw)
	require.True(t, ok)
	assert.Equal(t, "chunk", g.Type)
	require.Len(t, g.Choices, 1)
	assert.Equal(t, "OK", g.Choices[0].Delta.Content)

	// Prove the old path is broken so we never regress.
	broken, err := json.Marshal(raw)
	require.NoError(t, err)
	var bad guestStreamChunk
	err = json.Unmarshal(broken, &bad)
	require.Error(t, err, "json.Marshal([]byte) must not be used as decode path")
}

func TestDecodeGuestChunk_Done(t *testing.T) {
	raw := []byte(`{"type":"done","usage":{"prompt_tokens":1,"completion_tokens":2}}`)
	g, ok := decodeGuestChunk(raw)
	require.True(t, ok)
	assert.Equal(t, "done", g.Type)
	require.NotNil(t, g.Usage)
	assert.Equal(t, 1, g.Usage.PromptTokens)
}

func TestEmitGuestStreamChunk_MissingTypeWithChoices(t *testing.T) {
	ch := make(chan shared.StreamChunk, 1)
	ok := emitGuestStreamChunk(ch, guestStreamChunk{
		Choices: []guestChoice{{Delta: guestDelta{Content: "HELLO"}, Index: 0}},
	})
	require.True(t, ok)
	sc := <-ch
	assert.Equal(t, shared.ChunkText, sc.Type)
	assert.Equal(t, "HELLO", sc.Delta)
}

