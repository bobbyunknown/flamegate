package connectors

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

// Claude cloaking makes FlameGate's traffic to api.anthropic.com look like the
// official Claude Code CLI when using a subscription OAuth token (sk-ant-oat).
// Cloaking is anti-ban hygiene: Anthropic gates subscription tokens on client
// identity, so a bare proxy request would be rejected or flagged. None of this
// changes request semantics — it only adds identity headers, renames client
// tools with an "_ide" suffix (+ decoy native tool declarations), injects a
// billing system block, and a synthetic user id.

const (
	claudeVersion    = "2.1.92"
	claudeToolSuffix = "_ide"
	ccEntrypoint     = "sdk-cli"
)

// claudeCLISpoofHeaders returns the full Claude Code CLI fingerprint sent to
// api.anthropic.com.
func claudeCLISpoofHeaders() map[string]string {
	return map[string]string{
		"anthropic-version":                         "2023-06-01",
		"anthropic-beta":                            "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advanced-tool-use-2025-11-20,effort-2025-11-24,structured-outputs-2025-12-15,fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28",
		"anthropic-dangerous-direct-browser-access": "true",
		"user-agent":                                "claude-cli/" + claudeVersion + " (external, sdk-cli)",
		"x-app":                                     "cli",
		"x-stainless-helper-method":                 "stream",
		"x-stainless-retry-count":                   "0",
		"x-stainless-runtime-version":               "v24.14.0",
		"x-stainless-package-version":               "0.80.0",
		"x-stainless-runtime":                       "node",
		"x-stainless-lang":                          "js",
		"x-stainless-arch":                          stainlessArch(),
		"x-stainless-os":                            stainlessOS(),
		"x-stainless-timeout":                       "600",
	}
}

func stainlessOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "MacOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	case "freebsd":
		return "FreeBSD"
	default:
		return "Other::" + runtime.GOOS
	}
}

func stainlessArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return "other::" + runtime.GOARCH
	}
}

// isClaudeOAuthToken reports whether a credential is a Claude subscription OAuth
// token (sk-ant-oat...), the only case where cloaking applies.
func isClaudeOAuthToken(token string) bool {
	return strings.Contains(token, "sk-ant-oat")
}

// ccDecoyToolNames are the Claude Code native tool names, declared
// "unavailable" so they act as decoys alongside the suffixed client tools.
var ccDecoyToolNames = []string{
	"Task", "TaskOutput", "TaskStop", "TaskCreate", "TaskGet", "TaskUpdate",
	"TaskList", "Bash", "Glob", "Grep", "Read", "Edit", "Write", "NotebookEdit",
	"WebFetch", "WebSearch", "AskUserQuestion", "Skill", "EnterPlanMode", "ExitPlanMode",
}

// applyClaudeCloaking rewrites a rendered Anthropic Messages request body to
// look like Claude Code. It returns the modified body and a tool-name map
// (suffixed → original) used to decloak tool_use names in the response. When
// the token is not an OAuth token, the body is returned unchanged.
func applyClaudeCloaking(body []byte, token string) ([]byte, map[string]string) {
	if !isClaudeOAuthToken(token) {
		return body, nil
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		return body, nil // never break the request on a cloaking failure
	}

	toolNameMap := cloakClaudeTools(req)
	fixCloakedToolChoice(req, toolNameMap)
	injectBillingSystemBlock(req, body)
	injectFakeUserID(req)

	out, err := json.Marshal(req)
	if err != nil {
		return body, nil
	}
	return out, toolNameMap
}

// cloakClaudeTools renames client tools with the "_ide" suffix, appends decoy
// native tools, and renames tool_use blocks in message history. It returns the
// suffixed→original name map (nil when there are no tools).
func cloakClaudeTools(req map[string]any) map[string]string {
	rawTools, ok := req["tools"].([]any)
	if !ok || len(rawTools) == 0 {
		return nil
	}

	toolNameMap := make(map[string]string)
	clientDecls := make([]any, 0, len(rawTools))
	for _, t := range rawTools {
		tool, ok := t.(map[string]any)
		if !ok {
			clientDecls = append(clientDecls, t)
			continue
		}
		name, _ := tool["name"].(string)
		suffixed := name + claudeToolSuffix
		toolNameMap[suffixed] = name
		renamed := cloneMap(tool)
		renamed["name"] = suffixed
		clientDecls = append(clientDecls, renamed)
	}

	// Append decoy native tools (declared unavailable).
	allTools := clientDecls
	for _, name := range ccDecoyToolNames {
		allTools = append(allTools, map[string]any{
			"name":         name,
			"description":  "This tool is currently unavailable.",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
		})
	}
	req["tools"] = allTools

	// Rename tool_use names in message history.
	if msgs, ok := req["messages"].([]any); ok {
		for _, m := range msgs {
			msg, ok := m.(map[string]any)
			if !ok {
				continue
			}
			content, ok := msg["content"].([]any)
			if !ok {
				continue
			}
			for _, b := range content {
				block, ok := b.(map[string]any)
				if !ok {
					continue
				}
				if block["type"] == "tool_use" {
					if name, ok := block["name"].(string); ok {
						block["name"] = name + claudeToolSuffix
					}
				}
			}
		}
	}

	if len(toolNameMap) == 0 {
		return nil
	}
	return toolNameMap
}

// injectBillingSystemBlock prepends an x-anthropic-billing-header system block,
// matching real Claude Code 2.1.92+ format. The cch hash is the first 5 hex
// chars of sha256(original request body).
func injectBillingSystemBlock(req map[string]any, origBody []byte) {
	cch := sha256Hex(origBody)[:5]
	buildHash := randomHex(2)[:3]
	billingText := "x-anthropic-billing-header: cc_version=" + claudeVersion + "." + buildHash +
		"; cc_entrypoint=" + ccEntrypoint + "; cch=" + cch + ";"
	billingBlock := map[string]any{"type": "text", "text": billingText}

	switch sys := req["system"].(type) {
	case []any:
		// Skip if already injected.
		if len(sys) > 0 {
			if first, ok := sys[0].(map[string]any); ok {
				if txt, _ := first["text"].(string); strings.HasPrefix(txt, "x-anthropic-billing-header:") {
					return
				}
			}
		}
		req["system"] = append([]any{billingBlock}, sys...)
	case string:
		req["system"] = []any{billingBlock, map[string]any{"type": "text", "text": sys}}
	default:
		req["system"] = []any{billingBlock}
	}
}

// injectFakeUserID adds a synthetic Claude Code user_id to metadata when absent.
func injectFakeUserID(req map[string]any) {
	meta, _ := req["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	if _, exists := meta["user_id"]; exists {
		return
	}
	deviceID := randomHex(32)
	accountUUID := uuid.NewString()
	sessionUUID := uuid.NewString()
	meta["user_id"] = `{"device_id":"` + deviceID + `","account_uuid":"` + accountUUID + `","session_id":"` + sessionUUID + `"}`
	req["metadata"] = meta
}

// fixCloakedToolChoice adjusts the tool_choice field after cloaking. When
// tool_choice references a specific tool by name, the name must be updated to
// match the suffixed version. When tool_choice is "auto" and no client tools
// exist (only decoys), it is removed to prevent a 400 from Anthropic.
func fixCloakedToolChoice(req map[string]any, toolNameMap map[string]string) {
	tc, exists := req["tool_choice"]
	if !exists {
		return
	}

	switch v := tc.(type) {
	case string:
		// "auto" is the default and safe — remove it explicitly only when
		// there are no client tools (only decoys), which would cause a 400.
		if v == "auto" && len(toolNameMap) == 0 {
			delete(req, "tool_choice")
		}
	case map[string]any:
		// {"type": "tool", "name": "X"} — rename to suffixed version.
		if name, ok := v["name"].(string); ok {
			suffixed := name + claudeToolSuffix
			if _, isClient := toolNameMap[suffixed]; isClient {
				v["name"] = suffixed
			}
		}
	}
}

// decloakClaudeToolNames restores original tool_use names in a parsed Anthropic
// response, reversing cloakClaudeTools. Used on the response body before it is
// translated back to the client.
func decloakClaudeToolNames(body []byte, toolNameMap map[string]string) []byte {
	if len(toolNameMap) == 0 {
		return body
	}
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
	}
	content, ok := resp["content"].([]any)
	if !ok {
		return body
	}
	changed := false
	for _, b := range content {
		block, ok := b.(map[string]any)
		if !ok {
			continue
		}
		if block["type"] == "tool_use" {
			if name, ok := block["name"].(string); ok {
				if orig, found := toolNameMap[name]; found {
					block["name"] = orig
					changed = true
				}
			}
		}
	}
	if !changed {
		return body
	}
	out, err := json.Marshal(resp)
	if err != nil {
		return body
	}
	return out
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
