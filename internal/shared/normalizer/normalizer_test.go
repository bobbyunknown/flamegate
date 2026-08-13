package normalizer

import (
	"encoding/json"
	"testing"

	"github.com/bobbyunknown/flamegate/internal/domain"
)

func TestSanitizeToolCallIDs_ValidPassthrough(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "call_msg0_tc0_Read",
						Name:      "Read",
						Arguments: json.RawMessage(`{"file_path":"/tmp/a.txt"}`),
					}},
				},
			},
			{
				Role: core.RoleTool,
				Content: []core.ContentPart{
					{Type: core.PartToolResult, ToolResult: &core.ToolResult{
						CallID:  "call_msg0_tc0_Read",
						Content: "file content",
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	if req.Messages[0].Content[0].ToolCall.ID != "call_msg0_tc0_Read" {
		t.Errorf("valid ID was modified: %s", req.Messages[0].Content[0].ToolCall.ID)
	}
}

func TestSanitizeToolCallIDs_InvalidCharsStripped(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "call.msg@0#tc$0",
						Name:      "Read",
						Arguments: json.RawMessage(`{}`),
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	got := req.Messages[0].Content[0].ToolCall.ID
	if got != "callmsg0tc0" {
		t.Errorf("expected 'callmsg0tc0', got %q", got)
	}
}

func TestSanitizeToolCallIDs_EmptyIDGenerated(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "",
						Name:      "WebSearch",
						Arguments: json.RawMessage(`{}`),
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	got := req.Messages[0].Content[0].ToolCall.ID
	if got == "" {
		t.Error("expected generated ID, got empty")
	}
	if !toolIDPattern.MatchString(got) {
		t.Errorf("generated ID %q doesn't match pattern", got)
	}
}

func TestSanitizeToolCallIDs_EmptyArgsNormalized(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "tc1",
						Name:      "Read",
						Arguments: json.RawMessage(""),
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	got := string(req.Messages[0].Content[0].ToolCall.Arguments)
	if got != "{}" {
		t.Errorf("expected '{}', got %q", got)
	}
}

func TestSanitizeToolCallIDs_MalformedArgsNormalized(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "tc1",
						Name:      "Read",
						Arguments: json.RawMessage("{invalid json"),
					}},
				},
			},
		},
	}
	SanitizeToolCallIDs(req)
	got := string(req.Messages[0].Content[0].ToolCall.Arguments)
	if got != "{}" {
		t.Errorf("expected '{}', got %q", got)
	}
}

func TestFixMissingToolResults_AlreadyPresent(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "Read"}},
				},
			},
			{
				Role: core.RoleUser,
				Content: []core.ContentPart{
					{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "tc1", Content: "ok"}},
				},
			},
		},
	}
	FixMissingToolResults(req)
	if len(req.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(req.Messages))
	}
}

func TestFixMissingToolResults_InsertsEmptyResults(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "Read"}},
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc2", Name: "Write"}},
				},
			},
			{
				Role: core.RoleUser,
				Content: []core.ContentPart{
					{Type: core.PartText, Text: "Continue"},
				},
			},
		},
	}
	FixMissingToolResults(req)

	// The next message should now have tool_result parts.
	next := req.Messages[1]
	resultIDs := collectToolResultIDs(next)
	if len(resultIDs) != 2 {
		t.Errorf("expected 2 tool results, got %d: %v", len(resultIDs), resultIDs)
	}
}

func TestFixMissingToolResults_NoNextMessage(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "Read"}},
				},
			},
		},
	}
	FixMissingToolResults(req)

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages (synthetic tool result added), got %d", len(req.Messages))
	}
	last := req.Messages[len(req.Messages)-1]
	if last.Role != core.RoleTool {
		t.Errorf("expected role 'tool', got %q", last.Role)
	}
	if len(last.Content) != 1 || last.Content[0].ToolResult == nil {
		t.Error("expected tool_result part in synthetic message")
	}
}

// A tool_use whose result was dropped by compaction — leaving an assistant text
// turn between the call and the next user turn — must still get a synthetic
// result inserted right after the assistant call turn. The previous "check i+1
// only" logic missed this and left the tool_use dangling, 400ing strict
// providers.
func TestFixMissingToolResults_DanglingBetweenAssistants(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "Read"}},
			}},
			// Tool reply was compacted away; an assistant text turn follows.
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartText, Text: "let me continue"},
			}},
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartText, Text: "ok"},
			}},
		},
	}
	FixMissingToolResults(req)

	// A tool-role message carrying the result for tc1 must sit right after the
	// first assistant turn (index 1).
	if len(req.Messages) != 4 {
		t.Fatalf("expected 4 messages (synthetic result inserted), got %d", len(req.Messages))
	}
	inserted := req.Messages[1]
	if inserted.Role != core.RoleTool {
		t.Fatalf("expected synthetic tool message at index 1, got role %q", inserted.Role)
	}
	if ids := collectToolResultIDs(inserted); len(ids) != 1 || ids[0] != "tc1" {
		t.Errorf("expected synthetic result for tc1, got %v", ids)
	}
}

// When the genuine result exists somewhere in the conversation — even if not in
// the message immediately after the call — no synthetic duplicate is inserted.
func TestFixMissingToolResults_AnsweredAnywhereNoDuplicate(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{ID: "tc1", Name: "Read"}},
			}},
			// Anthropic shape: the result rides inside a user message.
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartText, Text: "here you go"},
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{CallID: "tc1", Content: "data"}},
			}},
		},
	}
	FixMissingToolResults(req)

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages (no synthetic insert), got %d", len(req.Messages))
	}
	count := 0
	for _, m := range req.Messages {
		for _, id := range collectToolResultIDs(m) {
			if id == "tc1" {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one result for tc1, got %d", count)
	}
}

func TestApply_Idempotent(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{
				Role: core.RoleAssistant,
				Content: []core.ContentPart{
					{Type: core.PartToolCall, ToolCall: &core.ToolCall{
						ID:        "bad@id!",
						Name:      "Read",
						Arguments: json.RawMessage(""),
					}},
				},
			},
			{
				Role: core.RoleUser,
				Content: []core.ContentPart{
					{Type: core.PartText, Text: "Continue"},
				},
			},
		},
	}

	Apply(req)
	msgs1 := len(req.Messages)
	id1 := req.Messages[0].Content[0].ToolCall.ID

	Apply(req)
	msgs2 := len(req.Messages)
	id2 := req.Messages[0].Content[0].ToolCall.ID

	if msgs1 != msgs2 {
		t.Errorf("message count changed: %d vs %d", msgs1, msgs2)
	}
	if id1 != id2 {
		t.Errorf("ID changed between runs: %q vs %q", id1, id2)
	}
}
