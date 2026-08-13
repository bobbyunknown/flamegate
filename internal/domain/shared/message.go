// Package core defines FlameGate's provider-agnostic domain model.
//
// Every inbound request (OpenAI, Anthropic, Gemini, ...) is normalized into
// these canonical types by the transform layer, routed by the pipeline, and
// rendered back into the wire format the caller expects. Keeping a single
// internal representation means connectors and routing logic never need to
// know which dialect the client spoke.
package shared

import "encoding/json"

// Role identifies the author of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// PartType discriminates the kind of content carried by a ContentPart.
type PartType string

const (
	PartText       PartType = "text"
	PartImage      PartType = "image"
	PartAudio      PartType = "audio"
	PartToolCall   PartType = "tool_call"
	PartToolResult PartType = "tool_result"
	PartThinking   PartType = "thinking"
)

// ContentPart is a single piece of multimodal message content. A message body
// is an ordered slice of parts; most text-only messages carry exactly one.
type ContentPart struct {
	Type PartType `json:"type"`

	// Text holds plain text for PartText and PartThinking.
	Text string `json:"text,omitempty"`

	// Media carries non-text payloads (image, audio).
	Media *MediaPayload `json:"media,omitempty"`

	// ToolCall is set when Type == PartToolCall.
	ToolCall *ToolCall `json:"tool_call,omitempty"`

	// ToolResult is set when Type == PartToolResult.
	ToolResult *ToolResult `json:"tool_result,omitempty"`

	// Signature carries provider-specific opaque data for thinking/reasoning
	// blocks that must be echoed back on follow-up turns (e.g. Anthropic).
	Signature string `json:"signature,omitempty"`
}

// MediaPayload represents binary or referenced media content.
type MediaPayload struct {
	// MIMEType, e.g. "image/png", "audio/wav".
	MIMEType string `json:"mime_type"`
	// URL references remote media. Mutually exclusive with Data.
	URL string `json:"url,omitempty"`
	// Data holds base64-encoded inline media. Mutually exclusive with URL.
	Data string `json:"data,omitempty"`
}

// ToolCall is a model's request to invoke a tool/function.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult is the output returned to the model after a tool executes. The
// slimmer (token-compression) layer targets the Content of these results.
type ToolResult struct {
	CallID  string `json:"call_id"`
	Content string `json:"content"`
	IsError bool   `json:"is_error,omitempty"`
}

// Message is one turn in a conversation.
type Message struct {
	Role    Role          `json:"role"`
	Name    string        `json:"name,omitempty"`
	Content []ContentPart `json:"content"`
}

// TextContent returns the concatenated text of all PartText parts. Convenience
// for connectors and filters that operate on plain text.
func (m Message) TextContent() string {
	var out string
	for _, p := range m.Content {
		if p.Type == PartText {
			out += p.Text
		}
	}
	return out
}
