package cache

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestEntryKey_Deterministic(t *testing.T) {
	vec := []float32{0.1, 0.2, 0.3, 0.4, 0.5}
	k1 := entryKey("test:", vec)
	k2 := entryKey("test:", vec)
	require.Equal(t, k1, k2, "same vector must produce same key")
}

func TestEntryKey_DifferentVectors(t *testing.T) {
	k1 := entryKey("test:", []float32{1, 0, 0})
	k2 := entryKey("test:", []float32{0, 1, 0})
	require.NotEqual(t, k1, k2, "different vectors should produce different keys")
}

func TestCacheResponseRoundtrip(t *testing.T) {
	original := &core.ChatResponse{
		ID:           "resp-123",
		Model:        "gpt-4o",
		FinishReason: core.FinishStop,
		Message: core.Message{
			Role:    core.RoleAssistant,
			Content: []core.ContentPart{{Type: core.PartText, Text: "Hello, world!"}},
		},
		Usage: core.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded core.ChatResponse
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, original.ID, decoded.ID)
	require.Equal(t, original.Model, decoded.Model)
	require.Equal(t, original.FinishReason, decoded.FinishReason)
	require.Equal(t, "Hello, world!", decoded.Message.TextContent())
	require.Equal(t, original.Usage.TotalTokens, decoded.Usage.TotalTokens)
}

func TestCacheEntrySerialization(t *testing.T) {
	entry := Entry{
		Vector: []float32{0.1, 0.2, 0.3},
		Response: &core.ChatResponse{
			ID:    "test-id",
			Model: "claude-sonnet-4.5",
			Message: core.Message{
				Role:    core.RoleAssistant,
				Content: []core.ContentPart{{Type: core.PartText, Text: "cached"}},
			},
		},
		Model:    "claude-sonnet-4.5",
		StoredAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	// Simulate what RedisStore.Put does.
	vecJSON, err := json.Marshal(entry.Vector)
	require.NoError(t, err)
	respJSON, err := json.Marshal(entry.Response)
	require.NoError(t, err)

	// Simulate what RedisStore.Nearest does.
	var vec []float32
	require.NoError(t, json.Unmarshal(vecJSON, &vec))
	var resp core.ChatResponse
	require.NoError(t, json.Unmarshal(respJSON, &resp))

	require.Equal(t, entry.Vector, vec)
	require.Equal(t, entry.Response.ID, resp.ID)
	require.Equal(t, entry.Response.Model, resp.Model)
	require.Equal(t, "cached", resp.Message.TextContent())
}
