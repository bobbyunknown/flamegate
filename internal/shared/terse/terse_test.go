package terse

import (
	"strings"
	"testing"

	"github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestApply_Disabled(t *testing.T) {
	req := &core.ChatRequest{System: "original instructions"}
	Apply(req, Config{Enabled: false})
	require.Equal(t, "original instructions", req.System)
}

func TestApply_NilRequest(t *testing.T) {
	require.NotPanics(t, func() {
		Apply(nil, Config{Enabled: true})
	})
}

func TestApply_InjectsTerseContext(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	Apply(req, Config{Enabled: true})
	require.Contains(t, req.System, sentinel)
	require.Contains(t, req.System, "TERSE format")
	require.Contains(t, req.System, "Conversation")
	require.Contains(t, req.System, "hello")
}

func TestApply_SerializesTools(t *testing.T) {
	req := &core.ChatRequest{
		Tools: []core.Tool{
			{Name: "read_file", Description: "Read a file from disk"},
		},
	}
	Apply(req, Config{Enabled: true})
	require.Contains(t, req.System, sentinel)
	require.Contains(t, req.System, "Tools")
	require.Contains(t, req.System, "read_file")
}

func TestApply_PrependsToExistingSystem(t *testing.T) {
	req := &core.ChatRequest{
		System: "you are a helpful assistant",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hi"}}},
		},
	}
	Apply(req, Config{Enabled: true})

	require.Contains(t, req.System, sentinel)
	require.Contains(t, req.System, "you are a helpful assistant")
	// terse block must come first
	require.True(t, strings.Index(req.System, sentinel) < strings.Index(req.System, "you are a helpful assistant"))
}

func TestApply_Idempotent(t *testing.T) {
	req := &core.ChatRequest{
		System: "base",
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "test"}}},
		},
	}
	Apply(req, Config{Enabled: true})
	first := req.System
	Apply(req, Config{Enabled: true})
	require.Equal(t, first, req.System, "second apply must not change anything")
	require.Equal(t, 1, strings.Count(req.System, sentinel), "sentinel must appear exactly once")
}

func TestApply_EmptyMessagesAndTools(t *testing.T) {
	req := &core.ChatRequest{System: "base"}
	Apply(req, Config{Enabled: true})
	require.Contains(t, req.System, sentinel)
	require.Contains(t, req.System, "TERSE format")
	// No conversation or tools sections when empty
	require.NotContains(t, req.System, "Conversation")
	require.NotContains(t, req.System, "Tools")
}

func TestApply_MultiPartContent(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartText, Text: "let me check"},
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "call_1", Name: "search", Arguments: []byte(`{"q":"test"}`)}},
				},
			},
		},
	}
	Apply(req, Config{Enabled: true})
	require.Contains(t, req.System, "let me check")
	require.Contains(t, req.System, "tool_call")
	require.Contains(t, req.System, "search")
}

func TestApply_LevelLight_DirectiveOnly(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
		Tools: []core.Tool{
			{Name: "read_file", Description: "Read a file"},
		},
	}
	Apply(req, Config{Enabled: true, Level: LevelLight})
	require.Contains(t, req.System, sentinel)
	require.Contains(t, req.System, "TERSE format")
	// light skips serialization
	require.NotContains(t, req.System, "Conversation")
	require.NotContains(t, req.System, "Tools")
}

func TestApply_LevelMedium_SerializesAll(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "hello"}}},
		},
	}
	Apply(req, Config{Enabled: true, Level: LevelMedium})
	require.Contains(t, req.System, "Conversation")
	require.Contains(t, req.System, "hello")
}

func TestApply_LevelAggressive_StripsThinking(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartThinking, Text: "internal reasoning process"},
					{Type: core.PartText, Text: "here is the answer"},
				},
			},
		},
	}
	Apply(req, Config{Enabled: true, Level: LevelAggressive})
	require.Contains(t, req.System, "Conversation")
	require.Contains(t, req.System, "here is the answer")
	// thinking content must be stripped
	require.NotContains(t, req.System, "internal reasoning process")
}

func TestApply_DefaultLevel_IsMedium(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "ping"}}},
		},
	}
	// Level is empty (zero value) — should default to medium.
	Apply(req, Config{Enabled: true, Level: ""})
	require.Contains(t, req.System, "Conversation")
	require.Contains(t, req.System, "ping")
}

func TestValidLevel(t *testing.T) {
	for _, l := range []Level{LevelLight, LevelMedium, LevelAggressive} {
		require.True(t, ValidLevel(l), "expected %q to be valid", l)
	}
	require.False(t, ValidLevel("bogus"))
}
