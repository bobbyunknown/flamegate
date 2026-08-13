package normalizer

import (
	"testing"

	"github.com/bobbyunknown/flamegate/internal/domain"
)

func toolNames(req *core.ChatRequest) []string {
	out := make([]string, 0, len(req.Tools))
	for _, t := range req.Tools {
		out = append(out, t.Name)
	}
	return out
}

func has(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// TestDedupeStripsBuiltinWhenExaPresent verifies built-in web tools are dropped
// when the Exa MCP equivalents are present.
func TestDedupeStripsBuiltinWhenExaPresent(t *testing.T) {
	req := &core.ChatRequest{
		Tools: []core.Tool{
			{Name: "mcp__exa__web_search_exa"},
			{Name: "mcp__exa__web_fetch_exa"},
			{Name: "WebSearch"},
			{Name: "WebFetch"},
			{Name: "Read"},
		},
	}
	DedupeBuiltinTools(req)
	names := toolNames(req)
	if has(names, "WebSearch") || has(names, "WebFetch") {
		t.Errorf("built-in web tools should be stripped, got %v", names)
	}
	if !has(names, "Read") || !has(names, "mcp__exa__web_search_exa") {
		t.Errorf("non-overlapping tools must survive, got %v", names)
	}
}

// TestDedupeNoopWithoutTriggers verifies the built-ins are kept when no MCP
// equivalent is present.
func TestDedupeNoopWithoutTriggers(t *testing.T) {
	req := &core.ChatRequest{
		Tools: []core.Tool{{Name: "WebSearch"}, {Name: "WebFetch"}, {Name: "Read"}},
	}
	DedupeBuiltinTools(req)
	if len(req.Tools) != 3 {
		t.Errorf("expected no stripping without triggers, got %v", toolNames(req))
	}
}

// TestDedupeBrowserMcpRegex verifies the regex-based browser rule.
func TestDedupeBrowserMcpRegex(t *testing.T) {
	req := &core.ChatRequest{
		Tools: []core.Tool{
			{Name: "mcp__browsermcp__navigate"},
			{Name: "mcp__Claude_in_Chrome__open"},
			{Name: "mcp__Claude_in_Chrome__click"},
			{Name: "Bash"},
		},
	}
	DedupeBuiltinTools(req)
	names := toolNames(req)
	for _, n := range names {
		if n == "mcp__Claude_in_Chrome__open" || n == "mcp__Claude_in_Chrome__click" {
			t.Errorf("Claude_in_Chrome tools should be stripped, got %v", names)
		}
	}
	if !has(names, "Bash") || !has(names, "mcp__browsermcp__navigate") {
		t.Errorf("expected Bash and browsermcp to survive, got %v", names)
	}
}
