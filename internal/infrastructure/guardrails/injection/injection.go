package injection

import (
	"context"
	"sort"
	"strings"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
)

// Detector implements guardrails.Detector for prompt injection.
type Detector struct{}

// New returns the prompt-injection detector.
func New() *Detector { return &Detector{} }

// Name identifies this detector in policy config and audit logs.
func (Detector) Name() string { return "injection" }

// Inbound scans the request's flattened text for injection signatures. If
// any hit meets the policy's severity threshold the configured action is
// returned; if no action is set the engine defaults to "warn".
func (Detector) Inbound(_ context.Context, in *guardrails.InboundRequest, p guardrails.Policy) (*guardrails.Decision, error) {
	cfg := p.Injection
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	hits := Scan(in.FlatText)
	if len(hits) == 0 {
		return nil, nil
	}
	threshold := guardrails.Severity(cfg.SeverityThreshold)
	highest := mapSeverity(Highest(hits))
	if !guardrails.MeetsThreshold(highest, threshold) {
		// Below the configured threshold — caller wanted to ignore low/medium
		// hits. We still emit a log-only audit row so users can see what fired.
		return &guardrails.Decision{
			Action:   guardrails.ActionLogOnly,
			Severity: highest,
			Reason:   reasonFor(hits),
			Findings: toFindings(hits),
		}, nil
	}
	action := cfg.Action
	if action == "" {
		action = guardrails.ActionWarn
	}
	return &guardrails.Decision{
		Action:   action,
		Severity: highest,
		Reason:   reasonFor(hits),
		Findings: toFindings(hits),
	}, nil
}

// Outbound: injection detection runs only on the prompt side. Returning nil
// is correct: the engine treats nil as ActionAllow with no findings.
func (Detector) Outbound(_ context.Context, _ *guardrails.OutboundResponse, _ guardrails.Policy) (*guardrails.Decision, error) {
	return nil, nil
}

func mapSeverity(s Severity) guardrails.Severity {
	switch s {
	case SeverityHigh:
		return guardrails.SeverityHigh
	case SeverityMedium:
		return guardrails.SeverityMedium
	case SeverityLow:
		return guardrails.SeverityLow
	}
	return ""
}

// reasonFor builds a stable, human-readable summary: "injection: <names>".
func reasonFor(hits []Hit) string {
	names := make(map[string]struct{}, len(hits))
	for _, h := range hits {
		names[h.Pattern] = struct{}{}
	}
	list := make([]string, 0, len(names))
	for n := range names {
		list = append(list, n)
	}
	sort.Strings(list)
	return "injection: " + strings.Join(list, ", ")
}

func toFindings(hits []Hit) []guardrails.Finding {
	out := make([]guardrails.Finding, 0, len(hits))
	for _, h := range hits {
		out = append(out, guardrails.Finding{
			Entity:   "PROMPT_INJECTION:" + h.Pattern,
			Score:    score(h.Severity),
			Start:    h.Start,
			End:      h.End,
			Original: truncate(h.Text, 96),
		})
	}
	return out
}

func score(s Severity) float64 {
	switch s {
	case SeverityHigh:
		return 0.9
	case SeverityMedium:
		return 0.7
	case SeverityLow:
		return 0.5
	}
	return 0.0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
