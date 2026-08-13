package shared

import "testing"

func TestEstimateTokensFromChars(t *testing.T) {
	cases := []struct {
		chars int
		want  int
	}{
		{0, 0},
		{-5, 0},
		{1, 1},
		{4, 1},
		{5, 2},
		{100, 25},
	}
	for _, tc := range cases {
		if got := EstimateTokensFromChars(tc.chars); got != tc.want {
			t.Errorf("EstimateTokensFromChars(%d) = %d, want %d", tc.chars, got, tc.want)
		}
	}
}

func TestEstimatePromptTokens(t *testing.T) {
	if got := EstimatePromptTokens(nil); got != 0 {
		t.Errorf("nil request = %d, want 0", got)
	}

	req := &ChatRequest{
		System: "1234", // 4 chars -> 1 token
		Messages: []Message{
			{Role: RoleUser, Content: []ContentPart{{Type: PartText, Text: "12345678"}}}, // 8 chars
		},
	}
	// 12 chars total -> (12+3)/4 = 3 tokens
	if got := EstimatePromptTokens(req); got != 3 {
		t.Errorf("EstimatePromptTokens = %d, want 3", got)
	}
}
