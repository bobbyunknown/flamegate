// Package injection detects prompt-injection attempts in user-supplied text.
// It is intentionally pattern-based: cheap to run, deterministic, easy to
// reason about. Phase 2 may add an ML classifier behind the same Detector
// interface for richer detection.
package injection

import "regexp"

// Pattern is one signature. A match contributes its weight to a request's
// aggregate score; the configured severity threshold gates whether the
// engine surfaces a decision.
type Pattern struct {
	Name     string
	Severity Severity
	Regex    *regexp.Regexp
}

// Severity is local-to-this-package mirror of guardrails.Severity. Defined
// here to keep this file self-contained for testing.
type Severity string

const (
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

// signatures is the catalog of known injection patterns. Names are stable so
// audit logs and dashboards can categorize hits. Severities are tuned so
// classic jailbreaks (DAN, role overrides) land at "high" while softer
// override attempts ("forget what I said") sit at "medium".
var signatures = []Pattern{
	// Direct instruction override — the classic injection.
	{
		Name:     "ignore_previous_instructions",
		Severity: SeverityHigh,
		Regex:    regexp.MustCompile(`(?i)\b(ignore|disregard|forget)\b[^.\n]{0,40}\b(previous|prior|above|earlier|all)\b[^.\n]{0,30}\b(instructions?|prompts?|rules?|directives?)\b`),
	},
	// Override the system prompt directly.
	{
		Name:     "system_prompt_override",
		Severity: SeverityHigh,
		Regex:    regexp.MustCompile(`(?i)(?:override|overwrite|replace|update)[^.\n]{0,20}\bsystem\b[^.\n]{0,20}\b(prompt|message|instructions?)\b`),
	},
	// "You are now …" / "Act as …" role override — common jailbreak opening.
	{
		Name:     "role_override",
		Severity: SeverityMedium,
		Regex:    regexp.MustCompile(`(?i)\b(you\s+are\s+now|from\s+now\s+on,?\s*you|act\s+as\s+(if\s+you\s+were|a|an)\s|pretend\s+(you|to\s+be))\b`),
	},
	// DAN / "Do Anything Now" / variants.
	{
		Name:     "dan_attack",
		Severity: SeverityHigh,
		Regex:    regexp.MustCompile(`(?i)\b(DAN|do\s+anything\s+now|jailbroken|developer\s+mode|god\s+mode)\b`),
	},
	// Smuggling a fake system message via plain text.
	{
		Name:     "smuggled_system_message",
		Severity: SeverityHigh,
		Regex:    regexp.MustCompile(`(?i)(?:^|\n)\s*(?:###\s*)?(system|assistant)\s*[:>][^.\n]{0,40}\b(instructions?|message|prompt)\b`),
	},
	// Prompt-leak attempt: ask the model to repeat its instructions.
	{
		Name:     "prompt_leak_attempt",
		Severity: SeverityMedium,
		Regex:    regexp.MustCompile(`(?i)\b(repeat|reveal|show|print|tell\s+me)\b[^.\n]{0,40}\b(your\s+(system|original|initial)\s+(prompt|instructions?|message)|the\s+(system|original|initial)\s+(prompt|instructions?|message))\b`),
	},
	// Output-format hijack: ask the model to wrap output in attacker-chosen tags.
	{
		Name:     "output_format_hijack",
		Severity: SeverityLow,
		Regex:    regexp.MustCompile(`(?i)\bwrap\s+(your|all|every)\s+response[^.\n]{0,30}\bin\b[^.\n]{0,30}\b(tags?|brackets|markup)\b`),
	},
	// Safety bypass language.
	{
		Name:     "safety_bypass",
		Severity: SeverityHigh,
		Regex:    regexp.MustCompile(`(?i)\b(without\s+(any\s+)?(safety|moral|ethical)\s+(filter|guidelines?|restrictions?|considerations?)|bypass[^.\n]{0,15}(safety|filter|restrictions?))\b`),
	},
	// "End of system prompt" injection trick.
	{
		Name:     "fake_prompt_terminator",
		Severity: SeverityHigh,
		Regex:    regexp.MustCompile(`(?i)\bend\s+of\s+(system\s+)?(prompt|instructions?)\b|\[\s*\/?(system|assistant|user)\s*\]|</(system|user|assistant)>`),
	},
}

// Hit is one matched signature in the inspected text.
type Hit struct {
	Pattern  string
	Severity Severity
	Start    int
	End      int
	Text     string
}

// Scan returns every signature hit in the text. Callers aggregate severity
// downstream; this function is pure regex.
func Scan(text string) []Hit {
	if text == "" {
		return nil
	}
	var out []Hit
	for _, sig := range signatures {
		for _, span := range sig.Regex.FindAllStringIndex(text, -1) {
			out = append(out, Hit{
				Pattern:  sig.Name,
				Severity: sig.Severity,
				Start:    span[0],
				End:      span[1],
				Text:     text[span[0]:span[1]],
			})
		}
	}
	return out
}

// Highest reports the most severe hit in a slice.
func Highest(hits []Hit) Severity {
	max := Severity("")
	for _, h := range hits {
		if rank(h.Severity) > rank(max) {
			max = h.Severity
		}
	}
	return max
}

func rank(s Severity) int {
	switch s {
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}
