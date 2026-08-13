package shared

// Token estimation helpers. These use the common ~4 chars/token heuristic,
// which is accurate enough for client-side budgeting and for synthesizing a
// usage event when an upstream provider omits token counts from a stream.

// EstimateTokensFromChars approximates a token count from a character count.
// Returns 0 for non-positive input.
func EstimateTokensFromChars(chars int) int {
	if chars <= 0 {
		return 0
	}
	return (chars + 3) / 4
}

// EstimatePromptTokens approximates the prompt token count for a request over
// system text, message content, tool-call arguments, tool results, and tool
// definitions.
func EstimatePromptTokens(req *ChatRequest) int {
	if req == nil {
		return 0
	}
	chars := len(req.System)
	for _, m := range req.Messages {
		for _, part := range m.Content {
			chars += len(part.Text)
			if part.ToolCall != nil {
				chars += len(part.ToolCall.Arguments)
			}
			if part.ToolResult != nil {
				chars += len(part.ToolResult.Content)
			}
		}
	}
	for _, t := range req.Tools {
		chars += len(t.Name) + len(t.Description) + len(t.Parameters)
	}
	return EstimateTokensFromChars(chars)
}
