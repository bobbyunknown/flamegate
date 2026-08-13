package transform

import (
	"strings"
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	json "github.com/bobbyunknown/flamegate/internal/shared/fastjson"
)

func TestCleanGeminiToolSchemaStripsUnsupportedKeywords(t *testing.T) {
	raw := json.RawMessage(`{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type": "object",
		"additionalProperties": false,
		"properties": {
			"path": {"type": "string", "minLength": 1, "format": "uri"},
			"mode": {"const": "read"}
		},
		"required": ["path", "ghost"]
	}`)

	cleaned := cleanGeminiToolSchema(raw)

	var got map[string]any
	if err := json.Unmarshal(cleaned, &got); err != nil {
		t.Fatalf("cleaned schema is not valid JSON: %v", err)
	}

	if _, ok := got["$schema"]; ok {
		t.Errorf("$schema should be stripped: %s", cleaned)
	}
	if _, ok := got["additionalProperties"]; ok {
		t.Errorf("additionalProperties should be stripped: %s", cleaned)
	}

	props := got["properties"].(map[string]any)
	pathSchema := props["path"].(map[string]any)
	if _, ok := pathSchema["minLength"]; ok {
		t.Errorf("minLength should be stripped: %s", cleaned)
	}
	if _, ok := pathSchema["format"]; ok {
		t.Errorf("format should be stripped: %s", cleaned)
	}

	// const should have become an enum of strings with explicit type.
	modeSchema := props["mode"].(map[string]any)
	if _, ok := modeSchema["const"]; ok {
		t.Errorf("const should be converted to enum: %s", cleaned)
	}
	enum, ok := modeSchema["enum"].([]any)
	if !ok || len(enum) != 1 || enum[0] != "read" {
		t.Errorf("const should map to enum [\"read\"]: %s", cleaned)
	}

	// required must drop the entry with no matching property.
	required := got["required"].([]any)
	if len(required) != 1 || required[0] != "path" {
		t.Errorf("required should keep only path: %v", required)
	}
}

func TestCleanGeminiToolSchemaEmptyObjectGetsPlaceholder(t *testing.T) {
	cleaned := cleanGeminiToolSchema(json.RawMessage(`{"type":"object","properties":{}}`))
	var got map[string]any
	if err := json.Unmarshal(cleaned, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	props, ok := got["properties"].(map[string]any)
	if !ok || len(props) == 0 {
		t.Errorf("empty object schema should get a placeholder property: %s", cleaned)
	}
}

func TestCleanGeminiToolSchemaFlattensAnyOf(t *testing.T) {
	raw := json.RawMessage(`{
		"anyOf": [
			{"type": "null"},
			{"type": "object", "properties": {"q": {"type": "string"}}}
		]
	}`)
	cleaned := cleanGeminiToolSchema(raw)
	var got map[string]any
	if err := json.Unmarshal(cleaned, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := got["anyOf"]; ok {
		t.Errorf("anyOf should be flattened: %s", cleaned)
	}
	if got["type"] != "object" {
		t.Errorf("anyOf should resolve to the object branch: %s", cleaned)
	}
}

func TestSanitizeGeminiName(t *testing.T) {
	cases := map[string]string{
		"validName":             "validName",
		"mcp__server__tool":     "mcp__server__tool",
		"weird name!":           "weird_name_",
		"":                      "_unknown",
		"123start":              "_123start",
		strings.Repeat("a", 80): strings.Repeat("a", 64),
	}
	for in, want := range cases {
		if got := sanitizeGeminiName(in); got != want {
			t.Errorf("sanitizeGeminiName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestGeminiToolCallResponsePairing verifies a round trip keeps the function
// name on the response side matching the call, which Gemini requires.
func TestGeminiToolCallResponsePairing(t *testing.T) {
	req := &core.ChatRequest{
		Model: "gemini-2.5-pro",
		Tools: []core.Tool{{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`)}},
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: "weather?"}}},
			{Role: core.RoleAssistant, Content: []core.ContentPart{{
				Type:     core.PartToolCall,
				ToolCall: &core.ToolCall{ID: "call_get_weather", Name: "get_weather", Arguments: json.RawMessage(`{"city":"NYC"}`)},
			}}},
			{Role: core.RoleTool, Content: []core.ContentPart{{
				Type:       core.PartToolResult,
				ToolResult: &core.ToolResult{CallID: "call_get_weather", Content: `{"temp":20}`},
			}}},
		},
	}

	body, err := GeminiCodec{}.RenderRequest(req)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	var out gemRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var callName, respName string
	for _, c := range out.Contents {
		for _, p := range c.Parts {
			if p.FunctionCall != nil {
				callName = p.FunctionCall.Name
			}
			if p.FunctionResponse != nil {
				respName = p.FunctionResponse.Name
			}
		}
	}
	if callName == "" || respName == "" {
		t.Fatalf("expected both a functionCall and functionResponse, got call=%q resp=%q", callName, respName)
	}
	if callName != respName {
		t.Errorf("functionResponse name %q must match functionCall name %q", respName, callName)
	}
}
