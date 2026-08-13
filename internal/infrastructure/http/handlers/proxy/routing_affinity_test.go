package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestRequestAffinityKeyPrefersExplicitHeader(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("X-Conversation-ID", "thread-123")

	got := requestAffinityKey(r, &core.ChatRequest{})
	require.Contains(t, got, "header:x-conversation-id:")
	require.NotContains(t, got, "thread-123")
}

func TestRequestAffinityKeyUsesStableFirstUserFingerprint(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	base := &core.ChatRequest{
		Model: "xiaomi-tokenplan/mimo-v2.5",
		Metadata: core.RequestMetadata{
			APIKeyID:      "key-1",
			ClientKind:    "codex",
			SourceDialect: core.DialectOpenAI,
		},
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "start this task"}}},
		},
	}
	followUp := &core.ChatRequest{
		Model:    base.Model,
		Metadata: base.Metadata,
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "start this task"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{{Type: core.PartText, Text: "working"}}},
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "continue"}}},
		},
	}

	require.Equal(t, requestAffinityKey(r, base), requestAffinityKey(r, followUp))
}

func TestRequestAffinityKeySupportsPromptCacheAndMetadataUserID(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req := &core.ChatRequest{
		Extra: map[string]json.RawMessage{
			"prompt_cache_key": json.RawMessage(`"cache-abc"`),
		},
	}

	got := requestAffinityKey(r, req)
	require.Contains(t, got, "body:")
	require.NotContains(t, got, "cache-abc")

	req = &core.ChatRequest{
		Extra: map[string]json.RawMessage{
			"metadata": json.RawMessage(`{"user_id":"session-user-1"}`),
		},
	}
	got = requestAffinityKey(r, req)
	require.Contains(t, got, "body:")
	require.NotContains(t, got, "session-user-1")
}
