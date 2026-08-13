package normalizer

import (
	"regexp"

	"github.com/bobbyunknown/flamegate/internal/domain"
)

// Tool dedup strips built-in tools that an equivalent MCP tool supersedes,
// cutting tool-definition token bloat. A rule fires only when its trigger tools
// are present in the request, so the rules are safe to run for every client:
// the triggers (mcp__exa__*, mcp__tavily__*, mcp__browsermcp__*) are emitted
// only by clients that also ship the built-ins being stripped.

// toolMatcher matches a tool name either literally or by regular expression.
type toolMatcher struct {
	lit string
	re  *regexp.Regexp
}

func lit(name string) toolMatcher { return toolMatcher{lit: name} }
func pat(expr string) toolMatcher { return toolMatcher{re: regexp.MustCompile(expr)} }
func (m toolMatcher) matches(n string) bool {
	if m.re != nil {
		return m.re.MatchString(n)
	}
	return m.lit == n
}

type dedupRule struct {
	triggers []toolMatcher
	strip    []toolMatcher
}

// dedupRules mirror the built-in/MCP overlaps worth collapsing.
var dedupRules = []dedupRule{
	{
		// Exa MCP present → drop the built-in web tools (Exa is preferred).
		triggers: []toolMatcher{lit("mcp__exa__web_search_exa"), lit("mcp__exa__web_fetch_exa")},
		strip:    []toolMatcher{lit("WebSearch"), lit("WebFetch"), lit("mcp__workspace__web_fetch")},
	},
	{
		// Tavily MCP present → drop the built-in web tools.
		triggers: []toolMatcher{lit("mcp__tavily__tavily_search"), lit("mcp__tavily__tavily_extract")},
		strip:    []toolMatcher{lit("WebSearch"), lit("WebFetch"), lit("mcp__workspace__web_fetch")},
	},
	{
		// Browser MCP present → drop the duplicate in-Chrome connector tools.
		triggers: []toolMatcher{pat(`^mcp__browsermcp__`)},
		strip:    []toolMatcher{pat(`^mcp__Claude_in_Chrome__`)},
	},
}

// DedupeBuiltinTools removes built-in tools superseded by an equivalent MCP
// tool present in the same request. It is a no-op when no rule's triggers
// match, so it never strips a tool a client genuinely relies on.
func DedupeBuiltinTools(req *core.ChatRequest) {
	if req == nil || len(req.Tools) == 0 {
		return
	}

	strip := map[string]bool{}
	for _, rule := range dedupRules {
		triggered := false
		for _, t := range req.Tools {
			if anyMatch(rule.triggers, t.Name) {
				triggered = true
				break
			}
		}
		if !triggered {
			continue
		}
		for _, t := range req.Tools {
			if anyMatch(rule.strip, t.Name) {
				strip[t.Name] = true
			}
		}
	}
	if len(strip) == 0 {
		return
	}

	kept := req.Tools[:0]
	for _, t := range req.Tools {
		if strip[t.Name] {
			continue
		}
		kept = append(kept, t)
	}
	req.Tools = kept
}

func anyMatch(matchers []toolMatcher, name string) bool {
	for _, m := range matchers {
		if m.matches(name) {
			return true
		}
	}
	return false
}
