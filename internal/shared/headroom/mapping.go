package headroom

import (
	"encoding/json"
	"strings"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// openAIMessage is the minimal OpenAI chat-message shape exchanged with the
// Headroom proxy. FlameGate works on core.ChatRequest before any provider
// dialect is formed, so a single mapping (core.ChatRequest <-> OpenAI messages)
// suffices — no per-provider-dialect mapping is needed.
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Name    string `json:"name,omitempty"`
	// ToolCalls carries assistant tool/function call requests.
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
	// ToolCallID links a tool-result message back to the call it answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// openAIToolCall mirrors the OpenAI tool_calls entry shape.
type openAIToolCall struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

// openAIFunction is the function payload of a tool call.
type openAIFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// toOpenAIMessages maps a core.ChatRequest to the OpenAI message list sent to
// the proxy. A non-empty System becomes a leading role:"system" message so it
// is compressed alongside the conversation; the reverse mapping restores it as
// a leading system message (round-trips System through the proxy).
func toOpenAIMessages(req *core.ChatRequest) []openAIMessage {
	if req == nil {
		return nil
	}

	out := make([]openAIMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		out = append(out, openAIMessage{Role: string(core.RoleSystem), Content: req.System})
	}
	for i := range req.Messages {
		out = append(out, messageToOpenAI(req.Messages[i]))
	}
	return out
}

// messageToOpenAI converts a single canonical message. Text and thinking parts
// are concatenated into Content, tool calls map to ToolCalls, and a tool-result
// part maps to Content plus ToolCallID. The mapping is reversible by
// openAIToMessage for the part kinds the proxy understands.
func messageToOpenAI(m core.Message) openAIMessage {
	om := openAIMessage{Role: string(m.Role), Name: m.Name}

	var content strings.Builder
	for _, p := range m.Content {
		switch p.Type {
		case core.PartText, core.PartThinking:
			content.WriteString(p.Text)
		case core.PartToolCall:
			if p.ToolCall != nil {
				om.ToolCalls = append(om.ToolCalls, openAIToolCall{
					ID:   p.ToolCall.ID,
					Type: "function",
					Function: openAIFunction{
						Name:      p.ToolCall.Name,
						Arguments: p.ToolCall.Arguments,
					},
				})
			}
		case core.PartToolResult:
			if p.ToolResult != nil {
				om.ToolCallID = p.ToolResult.CallID
				content.WriteString(p.ToolResult.Content)
			}
		}
	}
	om.Content = content.String()
	return om
}

// fromOpenAIMessages maps proxy-returned OpenAI messages back to canonical
// messages. It is the inverse of toOpenAIMessages: a leading system message is
// preserved with RoleSystem, role/order/content are kept, tool calls become
// PartToolCall, and a message carrying a ToolCallID becomes a PartToolResult.
func fromOpenAIMessages(msgs []openAIMessage) []core.Message {
	if msgs == nil {
		return nil
	}

	out := make([]core.Message, 0, len(msgs))
	for _, om := range msgs {
		out = append(out, openAIToMessage(om))
	}
	return out
}

// openAIToMessage converts a single OpenAI message back to canonical form.
func openAIToMessage(om openAIMessage) core.Message {
	m := core.Message{Role: core.Role(om.Role), Name: om.Name}

	switch {
	case om.ToolCallID != "":
		// Tool-result message: content is the result payload.
		m.Content = append(m.Content, core.ContentPart{
			Type: core.PartToolResult,
			ToolResult: &core.ToolResult{
				CallID:  om.ToolCallID,
				Content: om.Content,
			},
		})
	case om.Content != "":
		m.Content = append(m.Content, core.ContentPart{
			Type: core.PartText,
			Text: om.Content,
		})
	}

	for _, tc := range om.ToolCalls {
		m.Content = append(m.Content, core.ContentPart{
			Type: core.PartToolCall,
			ToolCall: &core.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}

	return m
}
