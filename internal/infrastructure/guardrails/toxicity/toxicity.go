// Package toxicity is FlameGate's toxicity detector. It supports two engines
// selected per-policy via ToxicityConfig.Engine: "native" (default, offline
// keyword catalog covering id + en) and "openai" (OpenAI Moderation API).
// Both implement the same internal engine contract and produce findings
// with category-named entities (TOXICITY:hate, TOXICITY:harassment, ...).
package toxicity

import (
	"context"
	"sort"
	"strings"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
)

// Detector implements guardrails.Detector for toxicity.
type Detector struct {
	native engine
	openai engine
}

// Config configures the detector at construction time.
type Config struct {
	// OpenAI, if non-nil, enables the OpenAI Moderation engine. When the
	// policy selects engine="openai" but this is nil, the detector falls
	// back to native and surfaces no error (still safe to ship).
	OpenAI *OpenAIConfig
}

// New builds a toxicity detector. Native engine is always available; OpenAI
// engine is wired only when cfg.OpenAI is non-nil.
func New(cfg Config) *Detector {
	d := &Detector{native: newNativeEngine()}
	if cfg.OpenAI != nil {
		d.openai = newOpenAIEngine(*cfg.OpenAI)
	}
	return d
}

// Name identifies this detector in policy config and audit logs.
func (Detector) Name() string { return "toxicity" }

// Inbound scans the prompt and returns a decision when the configured
// threshold is exceeded for any allowed category.
func (d *Detector) Inbound(ctx context.Context, in *guardrails.InboundRequest, p guardrails.Policy) (*guardrails.Decision, error) {
	return d.scan(ctx, in.FlatText, p, guardrails.DirectionInbound)
}

// Outbound scans the LLM response. Same routing as inbound — toxicity reads
// the same policy config for both directions.
func (d *Detector) Outbound(ctx context.Context, out *guardrails.OutboundResponse, p guardrails.Policy) (*guardrails.Decision, error) {
	return d.scan(ctx, out.Text, p, guardrails.DirectionOutbound)
}

func (d *Detector) scan(ctx context.Context, text string, p guardrails.Policy, dir guardrails.Direction) (*guardrails.Decision, error) {
	cfg := p.Toxicity
	if cfg == nil || !cfg.Enabled || strings.TrimSpace(text) == "" {
		return nil, nil
	}

	eng := d.pickEngine(cfg.Engine)
	allowed := allowedCategories(cfg.Categories)
	threshold := cfg.Threshold
	if threshold <= 0 {
		threshold = 70 // 0–100 scale; matches the dashboard default slider value.
	}

	scores, err := eng.Score(ctx, text, allowed)
	if err != nil {
		// Engine errors (e.g. OpenAI 5xx) must not fail-closed and block real
		// traffic. Log via the engine's caller; here we just allow.
		return nil, err
	}
	hits := filterByThreshold(scores, threshold)
	if len(hits) == 0 {
		return nil, nil
	}

	action := cfg.Action
	if action == "" {
		action = guardrails.ActionWarn
	}
	dec := &guardrails.Decision{
		Action:   action,
		Severity: severityFor(hits),
		Reason:   reasonFor(hits),
		Findings: toFindings(hits),
	}
	_ = dir // direction is set by the engine wrapper; tracked here for future use.
	return dec, nil
}

func (d *Detector) pickEngine(name string) engine {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "openai":
		if d.openai != nil {
			return d.openai
		}
	}
	return d.native
}

// allowedCategories normalizes the policy's category list to a lookup set.
// An empty list means "all categories".
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

// filterByThreshold drops scores below threshold. The result is sorted by
// score descending for stable display.
func filterByThreshold(scores []CategoryScore, threshold int) []CategoryScore {
	out := scores[:0]
	for _, s := range scores {
		if s.Score >= threshold {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func severityFor(hits []CategoryScore) guardrails.Severity {
	max := 0
	for _, h := range hits {
		if h.Score > max {
			max = h.Score
		}
	}
	switch {
	case max >= 85:
		return guardrails.SeverityHigh
	case max >= 70:
		return guardrails.SeverityMedium
	default:
		return guardrails.SeverityLow
	}
}

func reasonFor(hits []CategoryScore) string {
	parts := make([]string, 0, len(hits))
	for _, h := range hits {
		parts = append(parts, h.Category)
	}
	return "toxicity: " + strings.Join(parts, ", ")
}

func toFindings(hits []CategoryScore) []guardrails.Finding {
	out := make([]guardrails.Finding, 0, len(hits))
	for _, h := range hits {
		out = append(out, guardrails.Finding{
			Entity: "TOXICITY:" + h.Category,
			Score:  float64(h.Score) / 100.0,
			Start:  -1,
			End:    -1,
		})
	}
	return out
}

// CategoryScore is one category's score on the 0–100 scale shared with the
// dashboard threshold slider. Engines normalize their internal score to this
// scale.
type CategoryScore struct {
	Category string
	Score    int
}

// engine is the common contract for both native and OpenAI scoring backends.
// It returns one score per *requested* category (or all categories when
// allowed is nil); categories with zero hits may be omitted.
type engine interface {
	Score(ctx context.Context, text string, allowed map[string]bool) ([]CategoryScore, error)
}
