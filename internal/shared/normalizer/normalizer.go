// Package normalizer validates and repairs tool call structures in chat
// requests before they are sent upstream. It ensures tool call IDs are
// Anthropic-compatible, arguments are valid JSON, and every tool_use has a
// matching tool_result.
//
// It runs once, globally, on the canonical request — before any dialect
// translation — so the same tool-call guarantees hold for every provider
// (OpenAI, Anthropic, Gemini, Kiro, ...). Strict providers reject a tool_use
// with no matching tool_result (Anthropic 400 "unexpected tool_use_id",
// Bedrock/Kiro TOOL_USE_RESULT_MISMATCH), and OpenAI requires the tool reply to
// immediately follow the assistant's tool_calls; normalizing here keeps all of
// them happy.
package normalizer

import (
	"fmt"
	"regexp"

	json "github.com/bobbyunknown/flamegate/internal/shared/fastjson"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// toolIDPattern matches the Anthropic tool_use.id character set.
var toolIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Apply runs all normalizers on the request in place. It is safe to call
// multiple times (idempotent).
func Apply(req *core.ChatRequest) {
	if req == nil {
		return
	}
	DedupeBuiltinTools(req)
	SanitizeToolCallIDs(req)
	FixMissingToolResults(req)
}

// SanitizeToolCallIDs walks every message and repairs tool call/result IDs
// that don't match the Anthropic-required pattern [a-zA-Z0-9_-]+.
// Invalid characters are stripped; if nothing remains, a deterministic ID
// is generated. Arguments are also normalized to valid JSON.
func SanitizeToolCallIDs(req *core.ChatRequest) {
	for mi := range req.Messages {
		for pi := range req.Messages[mi].Content {
			p := &req.Messages[mi].Content[pi]

			switch p.Type {
			case core.PartToolCall:
				if p.ToolCall == nil {
					continue
				}
				p.ToolCall.ID = sanitizeID(p.ToolCall.ID, mi, pi, p.ToolCall.Name)
				normalizeArguments(p.ToolCall)

			case core.PartToolResult:
				if p.ToolResult == nil {
					continue
				}
				p.ToolResult.CallID = sanitizeID(p.ToolResult.CallID, mi, pi, "")
			}
		}
	}
}

// FixMissingToolResults guarantees every assistant tool_use is answered by a
// matching tool_result. A dangling tool_use appears when client-side history
// compaction keeps an assistant message but drops the tool reply, or when a
// tool round is interrupted mid-flight; strict providers then reject the whole
// request. For each unanswered tool_use the function inserts a synthetic
// tool-role message holding an empty result immediately after the assistant
// turn — the position OpenAI requires and every other dialect accepts.
//
// An id is treated as already answered when a tool_result for it exists
// anywhere in the conversation: a tool-role message, or a tool_result block
// embedded in a user message (the shape Anthropic clients use). This prevents
// inserting a duplicate when the genuine result is present but not in the
// immediately following message.
func FixMissingToolResults(req *core.ChatRequest) {
	// Phase 1: collect every call id that already has a result, wherever it lives.
	answered := make(map[string]bool)
	for _, msg := range req.Messages {
		for _, id := range collectToolResultIDs(msg) {
			answered[id] = true
		}
	}

	// Phase 2: after each assistant turn, append a synthetic result for any of
	// its calls still unanswered, preserving call order and de-duplicating ids.
	out := make([]core.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		out = append(out, msg)
		if msg.Role != core.RoleAssistant {
			continue
		}

		var parts []core.ContentPart
		seen := make(map[string]bool)
		for _, id := range collectToolCallIDs(msg) {
			if answered[id] || seen[id] {
				continue
			}
			seen[id] = true
			answered[id] = true
			parts = append(parts, core.ContentPart{
				Type:       core.PartToolResult,
				ToolResult: &core.ToolResult{CallID: id, Content: ""},
			})
		}
		if len(parts) > 0 {
			out = append(out, core.Message{Role: core.RoleTool, Content: parts})
		}
	}

	req.Messages = out
}

// --- helpers ---

// sanitizeID strips characters not matching [a-zA-Z0-9_-]. If the result is
// empty, a deterministic ID is generated.
func sanitizeID(id string, msgIdx, partIdx int, toolName string) string {
	if id == "" {
		return generateID(msgIdx, partIdx, toolName)
	}
	cleaned := cleanID(id)
	if cleaned == "" {
		return generateID(msgIdx, partIdx, toolName)
	}
	return cleaned
}

// cleanID removes characters outside [a-zA-Z0-9_-].
func cleanID(id string) string {
	buf := make([]byte, 0, len(id))
	for i := 0; i < len(id); i++ {
		c := id[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			buf = append(buf, c)
		}
	}
	return string(buf)
}

// generateID creates a deterministic tool call ID from position + name.
func generateID(msgIdx, partIdx int, toolName string) string {
	if toolName != "" {
		return fmt.Sprintf("call_msg%d_tc%d_%s", msgIdx, partIdx, cleanID(toolName))
	}
	return fmt.Sprintf("call_msg%d_tc%d", msgIdx, partIdx)
}

// normalizeArguments ensures ToolCall.Arguments is valid JSON.
func normalizeArguments(tc *core.ToolCall) {
	if len(tc.Arguments) == 0 {
		tc.Arguments = json.RawMessage("{}")
		return
	}
	// Verify it's valid JSON.
	var raw any
	if err := json.Unmarshal(tc.Arguments, &raw); err != nil {
		tc.Arguments = json.RawMessage("{}")
	}
}

// collectToolCallIDs returns all tool call IDs from a message's content.
func collectToolCallIDs(msg core.Message) []string {
	var ids []string
	for _, p := range msg.Content {
		if p.Type == core.PartToolCall && p.ToolCall != nil && p.ToolCall.ID != "" {
			ids = append(ids, p.ToolCall.ID)
		}
	}
	return ids
}

// collectToolResultIDs returns all tool result call IDs from a message's content.
func collectToolResultIDs(msg core.Message) []string {
	var ids []string
	for _, p := range msg.Content {
		if p.Type == core.PartToolResult && p.ToolResult != nil && p.ToolResult.CallID != "" {
			ids = append(ids, p.ToolResult.CallID)
		}
	}
	return ids
}
