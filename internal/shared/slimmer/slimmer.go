// Package slimmer reduces input-token cost by compressing the bulky tool
// outputs (diffs, directory listings, search results, build logs, ...) that
// coding agents feed back to the model as tool results.
//
// It operates on the canonical core.ChatRequest before any format translation,
// so a single implementation benefits every provider dialect. Each Rule knows
// how to detect a particular kind of output and rewrite it more compactly. The
// transform is lossy-by-intent but information-preserving: the model still sees
// what changed, what matched, or what exists — just without redundant bytes.
//
// Safety contract: a Rule must never enlarge its input or panic. The engine
// double-checks the result size and silently keeps the original text whenever a
// rule errors, grows the content, or yields an empty string. A failed
// compression therefore can never corrupt or break a request.
package slimmer

import (
	"strings"

	"github.com/bobbyunknown/flamegate/internal/domain"
)

// Rule compresses a single tool-result payload.
type Rule interface {
	// Name is a stable identifier used in metrics and config (e.g. "git-diff").
	Name() string
	// Detect reports a confidence score in [0,1] that this rule applies to the
	// given content. Only the first ~1KB is passed as a cheap probe; the engine
	// selects the highest-scoring rule above the detection threshold.
	Detect(probe string) float64
	// Compress rewrites content. It must return the compressed form, or the
	// original (or an error) if it cannot help.
	Compress(content string) (string, error)
}

// detectThreshold is the minimum Detect score for a rule to be applied.
const detectThreshold = 0.5

// probeSize is how many leading bytes are shown to Rule.Detect.
const probeSize = 1024

// minCompressBytes skips compression for payloads too small to be worth it.
const minCompressBytes = 256

// Config controls the slimmer engine.
type Config struct {
	Enabled bool
	// Disabled lists rule names to skip even when Enabled.
	Disabled []string
	// FilterLevel controls source-code comment stripping intensity.
	// FilterNone (default) disables it for backward compatibility.
	FilterLevel FilterLevel
}

// Stats summarizes a compression pass over one request.
type Stats struct {
	BytesBefore int
	BytesAfter  int
	Hits        []Hit
}

// Hit records one successful per-payload compression.
type Hit struct {
	Rule  string
	Saved int
}

// Saved returns total bytes removed across all hits.
func (s Stats) Saved() int { return s.BytesBefore - s.BytesAfter }

// Engine applies a registry of rules to requests.
type Engine struct {
	rules []Rule
}

// NewEngine builds an engine from the given rules (order is irrelevant; the
// engine selects by Detect score).
func NewEngine(rules ...Rule) *Engine {
	return &Engine{rules: rules}
}

// Default returns an engine pre-loaded with the built-in rule set.
func Default() *Engine {
	return NewEngine(BuiltinRules()...)
}

// Compress walks every tool-result part in req and rewrites its content in
// place using the best-matching rule. It returns stats describing the pass, or
// nil when disabled or when nothing was changed.
func (e *Engine) Compress(req *core.ChatRequest, cfg Config) *Stats {
	if req == nil || !cfg.Enabled {
		return nil
	}
	disabled := toSet(cfg.Disabled)

	stats := &Stats{}
	changed := false

	for mi := range req.Messages {
		parts := req.Messages[mi].Content
		for pi := range parts {
			if parts[pi].Type != core.PartToolResult || parts[pi].ToolResult == nil {
				continue
			}
			// Never touch error results: their exact text often matters for the
			// model to recover, and they are usually small anyway.
			if parts[pi].ToolResult.IsError {
				continue
			}
			original := parts[pi].ToolResult.Content
			stats.BytesBefore += len(original)

			compressed, rule := e.compressOne(original, disabled)

			// Secondary pass: source code comment filtering after primary rule.
			if cfg.FilterLevel > FilterNone && len(compressed) > minCompressBytes {
				if filtered, err := stripComments(compressed, cfg.FilterLevel); err == nil &&
					len(filtered) < len(compressed) && filtered != "" {
					compressed = filtered
				}
			}

			stats.BytesAfter += len(compressed)

			if rule != "" && len(compressed) < len(original) {
				parts[pi].ToolResult.Content = compressed
				stats.Hits = append(stats.Hits, Hit{Rule: rule, Saved: len(original) - len(compressed)})
				changed = true
			} else if len(compressed) < len(original) {
				parts[pi].ToolResult.Content = compressed
				stats.Hits = append(stats.Hits, Hit{Rule: "source-code", Saved: len(original) - len(compressed)})
				changed = true
			}
		}
	}

	if !changed {
		return nil
	}
	return stats
}

// compressOne selects and applies the best rule for a single payload. It
// returns the (possibly unchanged) content and the name of the rule that fired
// (empty if none did).
func (e *Engine) compressOne(content string, disabled map[string]struct{}) (string, string) {
	if len(content) < minCompressBytes {
		return content, ""
	}

	probe := content
	if len(probe) > probeSize {
		probe = probe[:probeSize]
	}

	var best Rule
	var bestScore float64
	for _, r := range e.rules {
		if _, off := disabled[r.Name()]; off {
			continue
		}
		if score := r.Detect(probe); score > bestScore {
			best, bestScore = r, score
		}
	}
	if best == nil || bestScore < detectThreshold {
		return content, ""
	}

	out, err := best.Compress(content)
	// Safety net: reject failures, growth, and empty output.
	if err != nil || out == "" || len(out) >= len(content) {
		return content, ""
	}
	return out, best.Name()
}

func toSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[strings.TrimSpace(it)] = struct{}{}
	}
	return m
}
