package pipeline

import (
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// TestEstimateStreamUsage verifies the fallback usage estimate combines a
// prompt estimate from the request with a completion estimate from the streamed
// output length.
func TestEstimateStreamUsage(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "12345678"}}}, // 8 chars -> 2 tokens
		},
	}
	// completionChars = 40 -> (40+3)/4 = 10 tokens
	got := estimateStreamUsage(req, 40)
	if got.PromptTokens != 2 {
		t.Errorf("PromptTokens = %d, want 2", got.PromptTokens)
	}
	if got.CompletionTokens != 10 {
		t.Errorf("CompletionTokens = %d, want 10", got.CompletionTokens)
	}
	if got.TotalTokens != 12 {
		t.Errorf("TotalTokens = %d, want 12", got.TotalTokens)
	}
}

// TestEstimateStreamUsage_NoOutput verifies a request with no streamed output
// still reports the prompt estimate and a zero completion.
func TestEstimateStreamUsage_NoOutput(t *testing.T) {
	req := &core.ChatRequest{
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "1234"}}}, // 4 chars -> 1 token
		},
	}
	got := estimateStreamUsage(req, 0)
	if got.PromptTokens != 1 {
		t.Errorf("PromptTokens = %d, want 1", got.PromptTokens)
	}
	if got.CompletionTokens != 0 {
		t.Errorf("CompletionTokens = %d, want 0", got.CompletionTokens)
	}
	if got.TotalTokens != 1 {
		t.Errorf("TotalTokens = %d, want 1", got.TotalTokens)
	}
}
