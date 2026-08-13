// Package topics is FlameGate's topic-boundary detector. It enforces two
// modes against a free-form topic list configured per policy:
//
//   - block: the configured topics are off-limits. Any prompt that matches
//     one triggers the configured action (default warn).
//   - allow: only the configured topics are permitted. Any prompt that does
//     not match at least one triggers the configured action.
//
// Detection is layered:
//
//   - Layer 1 (always on): keyword + ngram match. Cheap, deterministic,
//     covers the majority of cases.
//   - Layer 2 (opt-in via engine="embedding"): cosine-similarity between the
//     prompt embedding and a pre-computed topic embedding. Catches semantic
//     paraphrases the keyword path misses. Requires an Embedder wired at
//     construction; if none is wired the detector silently falls back to
//     keyword-only so existing policies keep working.
package topics

import (
	"context"
	"sort"
	"strings"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
)

// Embedder is the topic-detector's view of an embedding model. It is a
// strict subset of cache.Embedder so the cache package can be wired in
// directly without an import cycle.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Detector implements guardrails.Detector for topic boundaries.
type Detector struct {
	embedder Embedder
	// vectorCache memoizes the embedding for each topic string so we don't
	// re-embed the same topic on every request. Keyed by the *normalized*
	// topic text. Phase 2: a per-policy cache keyed on topic-list hash would
	// avoid the per-topic map lookup, but this is plenty fast in practice.
	vectorCache *topicVectorCache
}

// Config configures the detector at construction time.
type Config struct {
	// Embedder enables the embedding path. If nil the detector runs in
	// keyword-only mode regardless of the policy's engine selection.
	Embedder Embedder
}

// New builds a topics detector.
func New(cfg Config) *Detector {
	return &Detector{
		embedder:    cfg.Embedder,
		vectorCache: newTopicVectorCache(),
	}
}

// Name identifies this detector in policy config and audit logs.
func (Detector) Name() string { return "topics" }

// Inbound scans the user prompt. Action defaults to warn when none is set.
func (d *Detector) Inbound(ctx context.Context, in *guardrails.InboundRequest, p guardrails.Policy) (*guardrails.Decision, error) {
	cfg := p.Topics
	if cfg == nil || !cfg.Enabled || len(cfg.Topics) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(in.FlatText) == "" {
		return nil, nil
	}

	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "block"
	}

	matches := d.matchTopics(ctx, in.FlatText, cfg)
	triggered := false
	switch mode {
	case "allow":
		// Allowlist: anything NOT matching the list is off-policy.
		triggered = len(matches) == 0
	default: // "block"
		triggered = len(matches) > 0
	}
	if !triggered {
		return nil, nil
	}

	action := cfg.Action
	if action == "" {
		action = guardrails.ActionWarn
	}
	return &guardrails.Decision{
		Action:   action,
		Severity: guardrails.SeverityMedium,
		Reason:   reasonFor(mode, matches, cfg.Topics),
		Findings: toFindings(mode, matches, cfg.Topics),
	}, nil
}

// Outbound: topics are evaluated on the prompt only. Returning nil is
// correct — the engine treats nil as ActionAllow.
func (Detector) Outbound(_ context.Context, _ *guardrails.OutboundResponse, _ guardrails.Policy) (*guardrails.Decision, error) {
	return nil, nil
}

// match is one topic that matched the prompt, with the highest-confidence
// score across both layers.
type match struct {
	Topic string
	// Layer is "keyword" or "embedding"; used in audit findings.
	Layer string
	Score float64
}

// matchTopics runs Layer 1 (keyword) for every topic and, if the embedder
// is wired and the policy opts in, Layer 2 (embedding similarity) for any
// topic that did not already match.
func (d *Detector) matchTopics(ctx context.Context, text string, cfg *guardrails.TopicsConfig) []match {
	normalized := strings.ToLower(text)
	out := make([]match, 0, len(cfg.Topics))
	embedRun := d.shouldUseEmbedding(cfg)

	var promptVec []float32
	threshold := cfg.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.6
	}

	for _, raw := range cfg.Topics {
		topic := strings.TrimSpace(raw)
		if topic == "" {
			continue
		}
		// Layer 1: keyword/ngram.
		if keywordHit(normalized, topic) {
			out = append(out, match{Topic: topic, Layer: "keyword", Score: 1.0})
			continue
		}
		// Layer 2: embedding similarity (only if not already a keyword hit).
		if !embedRun {
			continue
		}
		if promptVec == nil {
			vec, err := d.embedder.Embed(ctx, text)
			if err != nil || len(vec) == 0 {
				// Embedder failure is non-fatal: keyword results stand. We
				// stop trying the embedder for the remaining topics on this
				// request to avoid N error calls.
				embedRun = false
				continue
			}
			promptVec = vec
		}
		topicVec, err := d.vectorCache.get(ctx, d.embedder, topic)
		if err != nil || len(topicVec) == 0 {
			continue
		}
		sim := cosine(promptVec, topicVec)
		if sim >= threshold {
			out = append(out, match{Topic: topic, Layer: "embedding", Score: sim})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func (d *Detector) shouldUseEmbedding(cfg *guardrails.TopicsConfig) bool {
	if d.embedder == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Engine)) {
	case "embedding", "hybrid":
		return true
	case "", "keyword":
		// Default: keyword-only. The embedding path is opt-in because it
		// adds a network call to the request critical path.
		return false
	}
	return false
}

func reasonFor(mode string, matches []match, topics []string) string {
	if mode == "allow" && len(matches) == 0 {
		return "topics: off-topic (allowlist=" + strings.Join(topics, ", ") + ")"
	}
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		parts = append(parts, m.Topic)
	}
	return "topics: " + mode + " match: " + strings.Join(parts, ", ")
}

func toFindings(mode string, matches []match, topics []string) []guardrails.Finding {
	if mode == "allow" && len(matches) == 0 {
		return []guardrails.Finding{{
			Entity: "TOPIC:off_allowlist",
			Score:  1.0,
			Start:  -1,
			End:    -1,
		}}
	}
	out := make([]guardrails.Finding, 0, len(matches))
	for _, m := range matches {
		out = append(out, guardrails.Finding{
			Entity: "TOPIC:" + m.Layer + ":" + m.Topic,
			Score:  m.Score,
			Start:  -1,
			End:    -1,
		})
	}
	_ = topics
	return out
}
