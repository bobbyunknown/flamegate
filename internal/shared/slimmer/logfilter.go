package slimmer

import (
	"regexp"
	"strings"
)

// logCategory classifies a log line by severity.
type logCategory int

const (
	logInfo logCategory = iota
	logWarn
	logError
)

// Smart log dedup normalization patterns. Timestamps, UUIDs, hex values,
// large numbers, and paths are replaced with placeholder tokens so that
// repeated messages with varying identifiers collapse into a single entry.
var (
	reLogTimestamp = regexp.MustCompile(`^\d{4}[-/]\d{2}[-/]\d{2}[T ]\d{2}:\d{2}:\d{2}[.,]?\d*\s*`)
	reLogUUID      = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
	reLogHex       = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	reLogBigNum    = regexp.MustCompile(`\b\d{4,}\b`)
	reLogPath      = regexp.MustCompile(`/[\w./\-]+`)
)

// Log severity detection keywords. Error covers FATAL/PANIC/CRITICAL and
// friends — the most important lines in a log that must never be silently
// dropped as noise.
var (
	logErrorKeywords = []string{"error", "fatal", "panic", "critical", "alert", "emerg", "severe"}
	logWarnKeywords  = []string{"warn", "notice"}
)

// categorizeLogLine returns the severity bucket for a log line.
func categorizeLogLine(line string) logCategory {
	lower := strings.ToLower(line)
	for _, kw := range logErrorKeywords {
		if strings.Contains(lower, kw) {
			return logError
		}
	}
	for _, kw := range logWarnKeywords {
		if strings.Contains(lower, kw) {
			return logWarn
		}
	}
	return logInfo
}

// normalizeLogLine replaces variable parts (timestamps, UUIDs, hex, numbers,
// paths) with placeholder tokens for dedup-key purposes. The original line is
// kept for display.
func normalizeLogLine(line string) string {
	s := reLogTimestamp.ReplaceAllString(line, "<TS> ")
	s = reLogUUID.ReplaceAllString(s, "<UUID>")
	s = reLogHex.ReplaceAllString(s, "<HEX>")
	s = reLogBigNum.ReplaceAllString(s, "<NUM>")
	s = reLogPath.ReplaceAllString(s, "<PATH>")
	return s
}

// logBucket holds deduplicated lines within one severity category.
type logBucket struct {
	total    int
	unique   []string
	seenKeys map[string]struct{}
}

func newLogBucket() *logBucket {
	return &logBucket{seenKeys: make(map[string]struct{})}
}

func (b *logBucket) add(line string) {
	b.total++
	key := normalizeLogLine(line)
	if _, ok := b.seenKeys[key]; !ok {
		b.seenKeys[key] = struct{}{}
		b.unique = append(b.unique, line)
	}
}

// smartDedupLogRule replaces the basic dedupLogRule with a smarter version
// that normalizes variable parts for dedup and categorizes by severity.
type smartDedupLogRule struct{}

func (smartDedupLogRule) Name() string { return "dedup-log" }

func (smartDedupLogRule) Detect(probe string) float64 {
	nonEmpty := nonEmptyLines(probe)
	if len(nonEmpty) < dedupMinLines {
		return 0
	}

	// Check for log-like signals: timestamps at line start or severity keywords.
	hasTimestamp := false
	hasSeverity := false
	for _, l := range nonEmpty[:min(20, len(nonEmpty))] {
		if reLogTimestamp.MatchString(l) {
			hasTimestamp = true
		}
		lower := strings.ToLower(l)
		for _, kw := range logErrorKeywords {
			if strings.Contains(lower, kw) {
				hasSeverity = true
				break
			}
		}
		if hasSeverity {
			break
		}
		for _, kw := range logWarnKeywords {
			if strings.Contains(lower, kw) {
				hasSeverity = true
				break
			}
		}
	}

	if hasTimestamp && hasSeverity {
		return 0.65
	}
	if hasTimestamp || hasSeverity {
		return 0.55
	}

	// Fallback: check for repeated lines (original dedup behavior).
	prev := ""
	repeats := 0
	for _, l := range nonEmpty {
		if l == prev {
			repeats++
		}
		prev = l
	}
	if repeats > len(nonEmpty)/4 {
		return 0.5
	}

	return 0
}

// Compress categorizes and deduplicates log lines. Output format:
// summary → capped unique errors → capped unique warnings → repeated-line
// fallback for non-log content.
func (smartDedupLogRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")

	errBucket := newLogBucket()
	warnBucket := newLogBucket()
	infoBucket := newLogBucket()

	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		switch categorizeLogLine(l) {
		case logError:
			errBucket.add(l)
		case logWarn:
			warnBucket.add(l)
		default:
			infoBucket.add(l)
		}
	}

	// If no errors or warnings and info is small, fall back to simple dedup.
	if errBucket.total == 0 && warnBucket.total == 0 && len(infoBucket.unique) < dedupMinLines {
		return simpleDedup(content)
	}

	var out []string

	// Summary line.
	out = append(out, "Log Summary")
	out = append(out, formatBucketSummary("error", errBucket.total, len(errBucket.unique)))
	out = append(out, formatBucketSummary("warn", warnBucket.total, len(warnBucket.unique)))
	out = append(out, formatBucketSummary("info", infoBucket.total, len(infoBucket.unique)))
	out = append(out, "")

	// Errors (most actionable, show up to CapWarnings unique).
	if len(errBucket.unique) > 0 {
		out = append(out, "[ERRORS]")
		shown := errBucket.unique
		if len(shown) > CapWarnings {
			out = append(out, shown[:CapWarnings]...)
			out = append(out, "… "+itoa(len(shown)-CapWarnings)+" more unique errors")
		} else {
			out = append(out, shown...)
		}
		out = append(out, "")
	}

	// Warnings.
	if len(warnBucket.unique) > 0 {
		out = append(out, "[WARNINGS]")
		shown := warnBucket.unique
		if len(shown) > CapWarnings {
			out = append(out, shown[:CapWarnings]...)
			out = append(out, "… "+itoa(len(shown)-CapWarnings)+" more unique warnings")
		} else {
			out = append(out, shown...)
		}
	}

	return strings.Join(out, "\n"), nil
}

// formatBucketSummary produces a line like "   [error] 42 errors (7 unique)".
func formatBucketSummary(label string, total, unique int) string {
	return "   [" + label + "] " + itoa(total) + " " + label + "s (" + itoa(unique) + " unique)"
}

// simpleDedup collapses consecutive duplicate lines and blank-line streaks.
// This is the fallback for content that doesn't look like structured logs.
func simpleDedup(content string) (string, error) {
	lines := strings.Split(content, "\n")
	var out []string
	var prev string
	run := 0
	blankStreak := 0

	flush := func() {
		if run > 1 {
			out = append(out, "  ⟲ ×"+itoa(run))
		}
		run = 0
	}

	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			flush()
			prev = ""
			if blankStreak < 1 {
				out = append(out, l)
			}
			blankStreak++
			continue
		}
		blankStreak = 0
		if l == prev {
			run++
			continue
		}
		flush()
		out = append(out, l)
		prev = l
		run = 1
	}
	flush()
	return strings.Join(out, "\n"), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
