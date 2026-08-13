package transform

import (
	json "github.com/bobbyunknown/flamegate/internal/shared/fastjson"
)

// Tool-choice translation. The canonical request stores tool_choice in the
// OpenAI form (the string "auto"/"none"/"required" or an object
// {type:"function",function:{name}}). Inbound non-OpenAI dialects normalize
// their own shape into this form at parse time, and each upstream codec renders
// it back into the shape that provider expects. Keeping a single canonical form
// means a forced/!auto tool choice survives any cross-dialect hop instead of
// being silently dropped.

// claudeToolChoiceToOpenAI converts an Anthropic tool_choice value into the
// canonical OpenAI form. Returns nil when absent or unrecognized.
func claudeToolChoiceToOpenAI(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil //nolint:nilerr // best-effort parser
	}
	typ, _ := m["type"].(string)
	switch typ {
	case "auto":
		return "auto"
	case "none":
		return "none"
	case "any":
		return "required"
	case "tool":
		if name, _ := m["name"].(string); name != "" {
			return map[string]any{"type": "function", "function": map[string]any{"name": name}}
		}
	}
	return nil
}

// openAIToolChoiceToClaude converts the canonical OpenAI tool_choice into the
// Anthropic form. Returns nil to omit the field (Anthropic then defaults to
// "auto"), so an unrecognized value never leaks upstream as a 400.
func openAIToolChoiceToClaude(v any) any {
	switch t := v.(type) {
	case string:
		switch t {
		case "required":
			return map[string]any{"type": "any"}
		case "none":
			return map[string]any{"type": "none"}
		case "auto":
			return map[string]any{"type": "auto"}
		}
	case map[string]any:
		// OpenAI forced tool: {type:"function", function:{name}}. Checked before
		// the native pass-through because this shape also carries a `type`
		// ("function") that Anthropic rejects.
		if fn, ok := t["function"].(map[string]any); ok {
			if name, _ := fn["name"].(string); name != "" {
				return map[string]any{"type": "tool", "name": name}
			}
		}
		// Already Anthropic-native — only pass through the types it accepts.
		switch typ, _ := t["type"].(string); typ {
		case "auto", "any", "tool", "none":
			return t
		}
	}
	return nil
}

// openAIToolChoiceToGemini converts the canonical OpenAI tool_choice into a
// Gemini toolConfig.functionCallingConfig. sanitize brings a forced tool name
// into Gemini's allowed character set. Returns nil to omit the field (Gemini
// then defaults to AUTO).
func openAIToolChoiceToGemini(v any, sanitize func(string) string) map[string]any {
	mode := func(m string) map[string]any {
		return map[string]any{"functionCallingConfig": map[string]any{"mode": m}}
	}
	switch t := v.(type) {
	case string:
		switch t {
		case "none":
			return mode("NONE")
		case "required":
			return mode("ANY")
		case "auto":
			return mode("AUTO")
		}
	case map[string]any:
		if fn, ok := t["function"].(map[string]any); ok {
			if name, _ := fn["name"].(string); name != "" {
				return map[string]any{
					"functionCallingConfig": map[string]any{
						"mode":                 "ANY",
						"allowedFunctionNames": []string{sanitize(name)},
					},
				}
			}
		}
	}
	return nil
}
