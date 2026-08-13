// Package terse implements FlameGate's output-token saving mode using the
// TERSE serialization format (github.com/RudsonCarvalho/terse-go).
//
// When enabled, Apply serializes the request's messages and tool definitions
// into the compact TERSE format and injects them into the system prompt. This
// reduces input tokens by replacing verbose JSON-style message arrays with a
// token-efficient representation that LLMs can still parse naturally.
//
// This is a request-side transform: it only modifies the system prompt and
// runs before format translation so it applies uniformly across every provider
// dialect.
package terse

import (
	"encoding/json"
	"strings"

	tersego "github.com/RudsonCarvalho/terse-go"
	"github.com/bobbyunknown/flamegate/internal/domain"
)

// sentinel marks instruction text FlameGate injected, so Apply is idempotent
// across retries and chained fallbacks. It is an HTML-style comment that models
// ignore in their output.
const sentinel = "<!-- flamegate:terse -->"

// Level selects how aggressively terse mode transforms the request.
type Level string

const (
	// LevelLight injects the TERSE directive only — no message or tool
	// serialization. Useful when you want the model to be terser without
	// restructuring the context.
	LevelLight Level = "light"
	// LevelMedium (default) injects the directive and serializes messages
	// and tools into the compact TERSE format.
	LevelMedium Level = "medium"
	// LevelAggressive is like medium but also strips assistant thinking
	// content from the serialized context, yielding maximum compression.
	LevelAggressive Level = "aggressive"
)

// Config controls terse-mode behavior for a request.
type Config struct {
	Enabled bool
	Level   Level
}

// directive is the lightweight instruction prepended to the TERSE context block.
const directive = `The following context uses TERSE format (Token-Efficient Serialization).
Key: T=true, F=false, ~=null, {k:v}=object, [v]=array, #[keys]\\nrows=schema table.
Respond normally with full technical detail.`

// ValidLevel reports whether s is a recognized terse level.
func ValidLevel(s Level) bool {
	switch s {
	case LevelLight, LevelMedium, LevelAggressive:
		return true
	default:
		return false
	}
}

// Apply serializes the request's messages and tools into TERSE format and
// injects them into req.System when cfg.Enabled.
//
// The Level controls serialization scope:
//   - light:    injects the TERSE directive only (no message/tool serialization)
//   - medium:   directive + serialized messages + tools (default)
//   - aggressive: medium + strips assistant thinking content from serialization
//
// It is a no-op when disabled or when the instruction has already been applied
// (detected via the sentinel marker), so it is safe to call repeatedly across
// retries and fallback attempts. The injected block is prepended so the terse
// context takes precedence over any pre-existing system text.
func Apply(req *core.ChatRequest, cfg Config) {
	if req == nil || !cfg.Enabled {
		return
	}
	if strings.Contains(req.System, sentinel) {
		return
	}

	level := cfg.Level
	if level == "" {
		level = LevelMedium
	}

	parts := []string{sentinel, directive}

	// light: directive-only — skip message/tool serialization.
	if level != LevelLight {
		if len(req.Messages) > 0 {
			msgs := messagesToMaps(req.Messages, level == LevelAggressive)
			if serialized, err := tersego.Serialize(toAnySlice(msgs)); err == nil {
				parts = append(parts, "\n## Conversation\n"+serialized)
			}
		}

		if len(req.Tools) > 0 {
			tools := toolsToMaps(req.Tools)
			if serialized, err := tersego.Serialize(toAnySlice(tools)); err == nil {
				parts = append(parts, "\n## Tools\n"+serialized)
			}
		}
	}

	block := strings.Join(parts, "\n")
	if strings.TrimSpace(req.System) == "" {
		req.System = block
		return
	}
	req.System = block + "\n\n" + req.System
}

// messagesToMaps converts core.Message slices into a generic map structure
// suitable for TERSE serialization. When stripThinking is true, assistant
// thinking parts (extended-thinking blocks) are omitted to save tokens.
func messagesToMaps(msgs []core.Message, stripThinking bool) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		entry := map[string]any{
			"role": string(m.Role),
		}
		if m.Name != "" {
			entry["name"] = m.Name
		}
		// Flatten content parts to text or structured maps.
		parts := make([]any, 0, len(m.Content))
		for _, p := range m.Content {
			// In aggressive mode, skip thinking blocks entirely.
			if stripThinking && p.Type == core.PartThinking {
				continue
			}
			switch p.Type {
			case core.PartText:
				parts = append(parts, p.Text)
			case core.PartToolCall:
				tc := map[string]any{
					"type": "tool_call",
					"id":   p.ToolCall.ID,
					"name": p.ToolCall.Name,
				}
				var args any
				if err := json.Unmarshal(p.ToolCall.Arguments, &args); err == nil {
					tc["arguments"] = args
				}
				parts = append(parts, tc)
			case core.PartToolResult:
				tr := map[string]any{
					"type":    "tool_result",
					"call_id": p.ToolResult.CallID,
					"content": p.ToolResult.Content,
				}
				if p.ToolResult.IsError {
					tr["is_error"] = true
				}
				parts = append(parts, tr)
			case core.PartThinking:
				parts = append(parts, "[thinking]"+p.Text)
			}
		}
		if len(parts) == 1 {
			entry["content"] = parts[0]
		} else if len(parts) > 1 {
			entry["content"] = parts
		}
		out = append(out, entry)
	}
	return out
}

// toolsToMaps converts core.Tool slices into a generic map structure suitable
// for TERSE serialization.
func toolsToMaps(tools []core.Tool) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		entry := map[string]any{
			"name": t.Name,
		}
		if t.Description != "" {
			entry["description"] = t.Description
		}
		if len(t.Parameters) > 0 {
			var params any
			if err := json.Unmarshal(t.Parameters, &params); err == nil {
				entry["parameters"] = params
			}
		}
		out = append(out, entry)
	}
	return out
}

// toAnySlice converts a typed slice to []any for TERSE serialization compatibility.
func toAnySlice[T any](s []T) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}
