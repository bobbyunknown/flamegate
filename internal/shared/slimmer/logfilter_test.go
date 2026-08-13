package slimmer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCategorizeLogLine(t *testing.T) {
	cases := map[string]logCategory{
		"2024-01-15 ERROR failed to connect":   logError,
		"FATAL: database crashed":              logError,
		"PICNIC: not a real level":             logInfo,
		"2024-01-15 WARN slow query detected":  logWarn,
		"NOTICE: table vacuumed":               logWarn,
		"2024-01-15 INFO request completed":    logInfo,
		"just a plain log line":                logInfo,
		"CRITICAL: out of memory":              logError,
		"SEVERE: connection refused":           logError,
		"ALERT: disk space low":                logError,
		"EMERG: system unusable":               logError,
	}
	for line, want := range cases {
		got := categorizeLogLine(line)
		require.Equal(t, want, got, "categorizeLogLine(%q)", line)
	}
}

func TestNormalizeLogLine(t *testing.T) {
	cases := map[string]string{
		"2024-01-15T10:30:45 error connecting":    "<TS> error connecting",
		"request 550e8400-e29b-41d4-a716-446655440000": "request <UUID>",
		"address 0xDEADBEEF allocated":             "address <HEX> allocated",
		"user 12345 logged in":                     "user <NUM> logged in",
		"reading /var/log/app/server.log":          "reading <PATH>",
	}
	for input, want := range cases {
		got := normalizeLogLine(input)
		require.Contains(t, got, want, "normalizeLogLine(%q) should contain %q, got %q", input, want, got)
	}
}

func TestSmartDedup_Categorizes(t *testing.T) {
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, "2024-01-15T10:30:45 ERROR connection refused to database")
	}
	for i := 0; i < 3; i++ {
		lines = append(lines, "2024-01-15T10:30:46 WARN slow query on table users")
	}
	for i := 0; i < 10; i++ {
		lines = append(lines, "2024-01-15T10:30:47 INFO request processed successfully")
	}

	content := strings.Join(lines, "\n")
	r := smartDedupLogRule{}
	result, err := r.Compress(content)
	require.NoError(t, err)
	require.Contains(t, result, "Log Summary")
	require.Contains(t, result, "[ERRORS]")
	require.Contains(t, result, "[WARNINGS]")
	require.Contains(t, result, "unique")
}

func TestSmartDedup_NormalizesTimestamps(t *testing.T) {
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, "2024-01-15T10:30:45 ERROR connection refused to database")
	}
	content := strings.Join(lines, "\n")

	r := smartDedupLogRule{}
	result, err := r.Compress(content)
	require.NoError(t, err)
	// All identical after normalization → 1 unique.
	require.Contains(t, result, "1 unique")
}

func TestSmartDedup_Detect(t *testing.T) {
	r := smartDedupLogRule{}

	// Content with timestamps and severity → high score.
	logContent := "2024-01-15T10:30:45 ERROR something failed\n" +
		"2024-01-15T10:30:46 WARN slow query\n" +
		"2024-01-15T10:30:47 INFO ok\n" +
		"2024-01-15T10:30:48 INFO ok\n" +
		"2024-01-15T10:30:49 INFO ok\n"
	require.Greater(t, r.Detect(logContent), 0.5)

	// Content without log signals → low or zero score.
	require.Equal(t, 0.0, r.Detect("hello world\n"))
}

func TestSimpleDedup_Fallback(t *testing.T) {
	line := "this is a repeated log message that should be deduplicated simply\n"
	content := strings.Repeat(line, 20)

	result, err := simpleDedup(content)
	require.NoError(t, err)
	require.Less(t, len(result), len(content))
	require.Contains(t, result, "×")
}

func TestLogBucket_Add(t *testing.T) {
	b := newLogBucket()
	b.add("2024-01-15T10:30:45 ERROR connection refused")
	b.add("2024-01-15T10:30:46 ERROR connection refused")
	b.add("2024-01-15T10:30:47 ERROR different error entirely")

	require.Equal(t, 3, b.total)
	require.Equal(t, 2, len(b.unique), "two unique after normalization")
}
