package proxy

import (
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestEstimateInputTokens(t *testing.T) {
	req := &core.ChatRequest{
		System: "be brief", // 8 chars
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{
				{Type: core.PartText, Text: "hi there"}, // 8 chars
			}},
		},
	}
	// 8 + 8 = 16 chars → ceil(16/4) = 4 tokens.
	if got := estimateInputTokens(req); got != 4 {
		t.Errorf("estimateInputTokens = %d, want 4", got)
	}
}

func TestEstimateInputTokens_ToolsAndResults(t *testing.T) {
	req := &core.ChatRequest{
		Tools: []core.Tool{
			{Name: "Read", Description: "read file", Parameters: []byte(`{"x":1}`)},
		},
		Messages: []core.Message{
			{Role: core.RoleTool, Content: []core.ContentPart{
				{Type: core.PartToolResult, ToolResult: &core.ToolResult{Content: "file body"}},
			}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{
				{Type: core.PartToolCall, ToolCall: &core.ToolCall{Arguments: []byte(`{"a":1}`)}},
			}},
		},
	}
	// Tool: 4 (Read) + 9 (read file) + 7 ({"x":1}) = 20
	// ToolResult content: 9 (file body)
	// ToolCall args: 7 ({"a":1})
	// total = 36 chars → ceil(36/4) = 9 tokens.
	if got := estimateInputTokens(req); got != 9 {
		t.Errorf("estimateInputTokens = %d, want 9", got)
	}
}

func TestEstimateInputTokens_NilAndEmpty(t *testing.T) {
	if got := estimateInputTokens(nil); got != 0 {
		t.Errorf("nil request = %d, want 0", got)
	}
	if got := estimateInputTokens(&core.ChatRequest{}); got != 0 {
		t.Errorf("empty request = %d, want 0", got)
	}
}
