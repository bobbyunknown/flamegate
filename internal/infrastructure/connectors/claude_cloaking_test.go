package connectors

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyClaudeCloaking_SkipsNonOAuth(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4.5","messages":[],"tools":[{"name":"foo","input_schema":{}}]}`)
	out, m := applyClaudeCloaking(body, "sk-ant-api03-plainkey")
	if m != nil {
		t.Error("non-OAuth token must not produce a tool map")
	}
	if string(out) != string(body) {
		t.Error("non-OAuth body must be unchanged")
	}
}

func TestApplyClaudeCloaking_OAuth(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4.5",
		"system": "be precise",
		"messages": [
			{"role": "assistant", "content": [{"type": "tool_use", "name": "foo", "id": "t1", "input": {}}]}
		],
		"tools": [{"name": "foo", "description": "d", "input_schema": {"type": "object", "properties": {}}}]
	}`)

	out, m := applyClaudeCloaking(body, "sk-ant-oat01-token")
	if m == nil {
		t.Fatal("OAuth token must produce a tool name map")
	}
	if m["foo_ide"] != "foo" {
		t.Errorf("tool map should map foo_ide→foo, got %v", m)
	}

	var req map[string]any
	if err := json.Unmarshal(out, &req); err != nil {
		t.Fatal(err)
	}

	// system[0] must be the billing block; original system preserved after it.
	sys, ok := req["system"].([]any)
	if !ok || len(sys) < 2 {
		t.Fatalf("system should be an array with billing block + original, got %v", req["system"])
	}
	first := sys[0].(map[string]any)
	if txt, _ := first["text"].(string); !strings.HasPrefix(txt, "x-anthropic-billing-header:") {
		t.Errorf("system[0] must be billing header, got %q", txt)
	}

	// tools: client tool renamed + decoys appended.
	tools := req["tools"].([]any)
	if len(tools) != 1+len(ccDecoyToolNames) {
		t.Errorf("expected %d tools, got %d", 1+len(ccDecoyToolNames), len(tools))
	}
	if name := tools[0].(map[string]any)["name"]; name != "foo_ide" {
		t.Errorf("client tool should be renamed to foo_ide, got %v", name)
	}

	// tool_use in history renamed.
	msgs := req["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].([]any)
	if name := content[0].(map[string]any)["name"]; name != "foo_ide" {
		t.Errorf("tool_use should be renamed to foo_ide, got %v", name)
	}

	// metadata.user_id injected as JSON with device_id/account_uuid/session_id.
	meta := req["metadata"].(map[string]any)
	uid, _ := meta["user_id"].(string)
	if !strings.Contains(uid, "device_id") || !strings.Contains(uid, "session_id") {
		t.Errorf("user_id should be Claude Code JSON, got %q", uid)
	}
}

func TestDecloakClaudeToolNames(t *testing.T) {
	m := map[string]string{"foo_ide": "foo"}
	body := []byte(`{"content":[{"type":"tool_use","name":"foo_ide","id":"t1"},{"type":"text","text":"hi"}]}`)
	out := decloakClaudeToolNames(body, m)

	var resp map[string]any
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatal(err)
	}
	content := resp["content"].([]any)
	if name := content[0].(map[string]any)["name"]; name != "foo" {
		t.Errorf("tool_use name should decloak to foo, got %v", name)
	}

	// No map → unchanged.
	if got := decloakClaudeToolNames(body, nil); string(got) != string(body) {
		t.Error("nil map must leave body unchanged")
	}
}

func TestClaudeSpoofHeaders(t *testing.T) {
	h := claudeCLISpoofHeaders()
	if !strings.HasPrefix(h["user-agent"], "claude-cli/") {
		t.Errorf("user-agent should spoof claude-cli, got %q", h["user-agent"])
	}
	if h["x-app"] != "cli" {
		t.Errorf("x-app should be cli, got %q", h["x-app"])
	}
	if !strings.Contains(h["anthropic-beta"], "claude-code-20250219") {
		t.Errorf("anthropic-beta should carry claude-code beta, got %q", h["anthropic-beta"])
	}
}
