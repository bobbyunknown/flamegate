package pii

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
)

// Detector is the guardrails.Detector implementation for PII.
type Detector struct {
	// presidio, when non-nil, handles policies that set engine="presidio".
	// Policies with engine="" or "native" always use the in-process Go
	// recognizers. The native path stays available as a fallback when the
	// sidecar errors out.
	presidio *PresidioEngine
}

// Config configures the detector. Pass a non-nil Presidio engine to enable
// engine="presidio" routing on a per-policy basis.
type Config struct {
	Presidio *PresidioEngine
}

// New returns the native-Go PII detector with no external engines wired.
func New() *Detector { return &Detector{} }

// NewWithConfig builds a detector with optional external engines.
func NewWithConfig(cfg Config) *Detector {
	return &Detector{presidio: cfg.Presidio}
}

// recognize dispatches a recognition call to the engine selected by the
// policy. The native path is used when engine is unset / "native", or when
// the Presidio engine returns an error (graceful fallback so a sidecar
// hiccup doesn't break safety).
func (d *Detector) recognize(ctx context.Context, text string, cfg *guardrails.PIIConfig, allowed map[Entity]bool, minScore float64) []Match {
	if d.presidio != nil && strings.EqualFold(cfg.Engine, "presidio") {
		matches, err := d.presidio.Recognize(ctx, text, allowed, minScore)
		if err == nil {
			return matches
		}
		// fall through to native on error
	}
	return Recognize(text, allowed, minScore)
}

// Name identifies this detector in audit logs and policy config.
func (*Detector) Name() string { return "pii" }

// Inbound scans the request's text content. When matches exceed the policy
// threshold, the configured strategy is applied: redact/replace/mask/hash
// rewrite the text in place, block returns a block decision the engine
// surfaces to the caller as policy_blocked.
func (d *Detector) Inbound(ctx context.Context, in *guardrails.InboundRequest, p guardrails.Policy) (*guardrails.Decision, error) {
	cfg := p.PII
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	allowed := allowedSet(cfg.Types)
	minScore := cfg.MinScore
	if minScore <= 0 {
		minScore = 0.5
	}

	// Run recognizers across the FlatText composed by the engine; this
	// covers System + Messages content with role delimiters.
	matches := d.recognize(ctx, in.FlatText, cfg, allowed, minScore)
	if len(matches) == 0 {
		return nil, nil
	}

	strategy := cfg.Strategy
	if strategy == "" {
		strategy = guardrails.PIIStrategyRedact
	}

	if strategy == guardrails.PIIStrategyBlock {
		return &guardrails.Decision{
			Action:   guardrails.ActionBlock,
			Severity: severityFor(matches),
			Reason:   buildReason("blocked", matches),
			Findings: toFindings(matches, nil),
		}, nil
	}

	// For mutating strategies we rewrite the most recent user message in
	// place. We work off the original user-message text (not the engine's
	// FlatText with role delimiters) so the rewritten payload looks natural
	// to the provider.
	userIdx := lastUserMessageIndex(in.Source)
	if userIdx < 0 {
		// No user message to rewrite — fall back to redacting the system
		// prompt only (covers the rare case of system-only prompts).
		original := in.Source.System
		rewritten, redactions := d.applyStrategy(ctx, original, cfg, allowed, minScore, strategy)
		if rewritten == original {
			return nil, nil
		}
		return &guardrails.Decision{
			Action:       guardrails.ActionMask,
			Severity:     severityFor(matches),
			Reason:       buildReason("masked", matches),
			Findings:     toFindings(matches, redactions),
			Mutated:      rewritten,
			MutatedField: guardrails.MutatedFieldSystem,
		}, nil
	}

	original := in.Source.Messages[userIdx].TextContent()
	rewritten, redactions := d.applyStrategy(ctx, original, cfg, allowed, minScore, strategy)
	if rewritten == original {
		// All matches were in non-user surfaces; nothing to mutate cleanly
		// in Phase 1 — surface a Warn so audit captures it.
		return &guardrails.Decision{
			Action:   guardrails.ActionWarn,
			Severity: severityFor(matches),
			Reason:   buildReason("warned", matches),
			Findings: toFindings(matches, nil),
		}, nil
	}

	return &guardrails.Decision{
		Action:       guardrails.ActionMask,
		Severity:     severityFor(matches),
		Reason:       buildReason("masked", matches),
		Findings:     toFindings(matches, redactions),
		Mutated:      rewritten,
		MutatedField: guardrails.MutatedFieldMessages,
	}, nil
}

// Outbound scans the LLM response for leaked PII when ScanOutput is set.
func (d *Detector) Outbound(ctx context.Context, out *guardrails.OutboundResponse, p guardrails.Policy) (*guardrails.Decision, error) {
	cfg := p.PII
	if cfg == nil || !cfg.Enabled || !cfg.ScanOutput {
		return nil, nil
	}
	allowed := allowedSet(cfg.Types)
	minScore := cfg.MinScore
	if minScore <= 0 {
		minScore = 0.5
	}
	matches := d.recognize(ctx, out.Text, cfg, allowed, minScore)
	if len(matches) == 0 {
		return nil, nil
	}
	strategy := cfg.Strategy
	if strategy == "" {
		strategy = guardrails.PIIStrategyRedact
	}
	if strategy == guardrails.PIIStrategyBlock {
		return &guardrails.Decision{
			Action:   guardrails.ActionBlock,
			Severity: severityFor(matches),
			Reason:   buildReason("output blocked", matches),
			Findings: toFindings(matches, nil),
		}, nil
	}
	rewritten, redactions := d.applyStrategy(ctx, out.Text, cfg, allowed, minScore, strategy)
	if rewritten == out.Text {
		return nil, nil
	}
	return &guardrails.Decision{
		Action:       guardrails.ActionMask,
		Severity:     severityFor(matches),
		Reason:       buildReason("output masked", matches),
		Findings:     toFindings(matches, redactions),
		Mutated:      rewritten,
		MutatedField: guardrails.MutatedFieldResponse,
	}, nil
}

// allowedSet converts the policy's slice of entity names to a lookup set.
// An empty list means "all entities", encoded as nil.
func allowedSet(types []string) map[Entity]bool {
	if len(types) == 0 {
		return nil
	}
	out := make(map[Entity]bool, len(types))
	for _, t := range types {
		out[Entity(strings.ToUpper(strings.TrimSpace(t)))] = true
	}
	return out
}

// lastUserMessageIndex returns the index of the most recent user-role
// message in the request, or -1 if none.
func lastUserMessageIndex(req *core.ChatRequest) int {
	if req == nil {
		return -1
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == core.RoleUser {
			return i
		}
	}
	return -1
}

// applyStrategy rewrites text by substituting each match per the chosen
// strategy. It scans the original text once and emits the rewritten string
// plus a parallel list of redacted replacements (for audit logs).
func (d *Detector) applyStrategy(ctx context.Context, text string, cfg *guardrails.PIIConfig, allowed map[Entity]bool, minScore float64, strategy guardrails.PIIStrategy) (string, []string) {
	matches := d.recognize(ctx, text, cfg, allowed, minScore)
	if len(matches) == 0 {
		return text, nil
	}
	// Sort by Start so we splice from earliest to latest.
	sort.Slice(matches, func(i, j int) bool { return matches[i].Start < matches[j].Start })

	var b strings.Builder
	b.Grow(len(text))
	cursor := 0
	redactions := make([]string, 0, len(matches))
	for _, m := range matches {
		if m.Start < cursor {
			// Overlap with already-emitted text (mergeOverlaps should prevent
			// this but be safe).
			continue
		}
		b.WriteString(text[cursor:m.Start])
		repl := replacement(m, strategy)
		b.WriteString(repl)
		redactions = append(redactions, repl)
		cursor = m.End
	}
	b.WriteString(text[cursor:])
	return b.String(), redactions
}

// replacement renders the substitute for one match under the strategy.
func replacement(m Match, strategy guardrails.PIIStrategy) string {
	switch strategy {
	case guardrails.PIIStrategyReplace, guardrails.PIIStrategyAnonymize:
		return "<" + string(m.Entity) + ">"
	case guardrails.PIIStrategyMask:
		return maskPreservingEdges(m.Text)
	case guardrails.PIIStrategyHash:
		sum := sha256.Sum256([]byte(m.Text))
		return "<" + string(m.Entity) + ":" + hex.EncodeToString(sum[:4]) + ">"
	default: // PIIStrategyRedact
		return "<PII>"
	}
}

// maskPreservingEdges keeps the first 2 and last 2 visible characters and
// replaces the middle with '*'. For short strings (<= 4 chars) everything is
// masked to a constant token.
func maskPreservingEdges(s string) string {
	runes := []rune(s)
	if len(runes) <= 4 {
		return "<MASKED>"
	}
	out := make([]rune, len(runes))
	for i := range runes {
		switch {
		case i < 2, i >= len(runes)-2:
			out[i] = runes[i]
		default:
			out[i] = '*'
		}
	}
	return string(out)
}

// severityFor maps the highest-scoring match to a severity label.
func severityFor(matches []Match) guardrails.Severity {
	max := 0.0
	for _, m := range matches {
		if m.Score > max {
			max = m.Score
		}
	}
	switch {
	case max >= 0.85:
		return guardrails.SeverityHigh
	case max >= 0.65:
		return guardrails.SeverityMedium
	default:
		return guardrails.SeverityLow
	}
}

// buildReason produces a human-readable summary for audit and warning headers.
func buildReason(verb string, matches []Match) string {
	counts := map[Entity]int{}
	for _, m := range matches {
		counts[m.Entity]++
	}
	parts := make([]string, 0, len(counts))
	for e, c := range counts {
		parts = append(parts, string(e)+" x"+itoa(c))
	}
	sort.Strings(parts)
	return "PII " + verb + ": " + strings.Join(parts, ", ")
}

// toFindings converts internal matches into guardrails.Finding records for
// audit. Redactions is optional; when non-nil it MUST be the same length as
// matches in the order returned by applyStrategy.
func toFindings(matches []Match, redactions []string) []guardrails.Finding {
	out := make([]guardrails.Finding, 0, len(matches))
	for i, m := range matches {
		f := guardrails.Finding{
			Entity:   string(m.Entity),
			Score:    m.Score,
			Start:    m.Start,
			End:      m.End,
			Original: truncate(m.Text, 64),
		}
		if i < len(redactions) {
			f.Redacted = redactions[i]
		}
		out = append(out, f)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
