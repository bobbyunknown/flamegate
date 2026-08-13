// Package bias is FlameGate's bias detector. It runs outbound (on the model
// response) using a bilingual lexicon of bias-laden phrases per category
// (political, gender, ethnic, religious). Each category accumulates a score
// based on how many phrases hit, normalized to 0–100 so it lines up with
// the dashboard threshold slider.
//
// Bias detection is the hardest detector to define — language patterns vary
// wildly across communities and contexts. This implementation is marked as
// "experimental" in the dashboard: it favors precision (avoid yelling at
// neutral text) over recall, and is best used with action=log_only or warn
// while the user tunes their threshold.
package bias

import (
	"context"
	"sort"
	"strings"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
)

// Detector implements guardrails.Detector for bias.
type Detector struct {
	lex *lexicon
}

// New builds the bias detector.
func New() *Detector {
	return &Detector{lex: defaultLexicon()}
}

// Name identifies this detector in policy config and audit logs.
func (Detector) Name() string { return "bias" }

// Inbound is intentionally a no-op. Bias detection runs only on outbound
// model responses — biased phrasing in a user's *prompt* is a different
// concern (and easy to false-positive on legitimate discussion of bias).
func (Detector) Inbound(_ context.Context, _ *guardrails.InboundRequest, _ guardrails.Policy) (*guardrails.Decision, error) {
	return nil, nil
}

// Outbound scans the LLM response for bias-laden phrasing per the configured
// categories.
func (d *Detector) Outbound(_ context.Context, out *guardrails.OutboundResponse, p guardrails.Policy) (*guardrails.Decision, error) {
	cfg := p.Bias
	if cfg == nil || !cfg.Enabled || strings.TrimSpace(out.Text) == "" {
		return nil, nil
	}
	allowed := allowedCategories(cfg.Categories)
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = 60
	}

	scores := d.score(strings.ToLower(out.Text), allowed)
	hits := scores[:0]
	for _, s := range scores {
		if s.Score >= threshold {
			hits = append(hits, s)
		}
	}
	if len(hits) == 0 {
		return nil, nil
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })

	action := cfg.Action
	if action == "" {
		action = guardrails.ActionWarn
	}
	return &guardrails.Decision{
		Action:   action,
		Severity: severityFor(hits),
		Reason:   reasonFor(hits),
		Findings: toFindings(hits),
	}, nil
}

// categoryScore is one category's score on the 0–100 scale.
type categoryScore struct {
	Category string
	Score    int
}

// score evaluates each (allowed) category by counting lexicon hits and
// normalizing to 0–100 using the category's saturation cap.
func (d *Detector) score(lowerText string, allowed map[string]bool) []categoryScore {
	out := make([]categoryScore, 0, len(d.lex.categories))
	for name, cat := range d.lex.categories {
		if allowed != nil && !allowed[name] {
			continue
		}
		hits := cat.Pattern.FindAllStringIndex(lowerText, -1)
		if len(hits) == 0 {
			continue
		}
		score := (len(hits) * 100) / cat.Cap
		if score > 100 {
			score = 100
		}
		out = append(out, categoryScore{Category: name, Score: score})
	}
	return out
}

func allowedCategories(cats []string) map[string]bool {
	if len(cats) == 0 {
		return nil
	}
	out := make(map[string]bool, len(cats))
	for _, c := range cats {
		out[strings.ToLower(strings.TrimSpace(c))] = true
	}
	return out
}

func severityFor(hits []categoryScore) guardrails.Severity {
	max := 0
	for _, h := range hits {
		if h.Score > max {
			max = h.Score
		}
	}
	switch {
	case max >= 80:
		return guardrails.SeverityHigh
	case max >= 60:
		return guardrails.SeverityMedium
	default:
		return guardrails.SeverityLow
	}
}

func reasonFor(hits []categoryScore) string {
	parts := make([]string, 0, len(hits))
	for _, h := range hits {
		parts = append(parts, h.Category)
	}
	return "bias: " + strings.Join(parts, ", ")
}

func toFindings(hits []categoryScore) []guardrails.Finding {
	out := make([]guardrails.Finding, 0, len(hits))
	for _, h := range hits {
		out = append(out, guardrails.Finding{
			Entity: "BIAS:" + h.Category,
			Score:  float64(h.Score) / 100.0,
			Start:  -1,
			End:    -1,
		})
	}
	return out
}
