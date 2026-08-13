package slimmer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/stretchr/testify/require"
)

// toolResultReq builds a request whose single message carries one tool result.
func toolResultReq(content string, isErr bool) *core.ChatRequest {
	return &core.ChatRequest{
		Messages: []core.Message{{
			Role: core.RoleTool,
			Content: []core.ContentPart{{
				Type:       core.PartToolResult,
				ToolResult: &core.ToolResult{CallID: "c1", Content: content, IsError: isErr},
			}},
		}},
	}
}

func resultContent(req *core.ChatRequest) string {
	return req.Messages[0].Content[0].ToolResult.Content
}

func TestEngine_DisabledIsNoop(t *testing.T) {
	big := strings.Repeat("line\n", 500)
	req := toolResultReq(big, false)
	stats := Default().Compress(req, Config{Enabled: false})
	require.Nil(t, stats)
	require.Equal(t, big, resultContent(req))
}

func TestEngine_SkipsErrorResults(t *testing.T) {
	// A diff that would normally compress, but flagged as an error result.
	diff := "diff --git a/x b/x\n" + strings.Repeat("@@ -1,1 +1,1 @@\n context\n", 200)
	req := toolResultReq(diff, true)
	stats := Default().Compress(req, Config{Enabled: true})
	require.Nil(t, stats, "error results must be left untouched")
	require.Equal(t, diff, resultContent(req))
}

func TestEngine_SkipsSmallPayloads(t *testing.T) {
	req := toolResultReq("tiny output", false)
	stats := Default().Compress(req, Config{Enabled: true})
	require.Nil(t, stats)
}

func TestEngine_NeverGrowsContent(t *testing.T) {
	// Whatever rule fires, output must be <= input.
	inputs := []string{
		"diff --git a/f b/f\n" + strings.Repeat("@@ -0,0 +1 @@\n+x\n", 300),
		strings.Repeat("src/a/b/c.go\n", 400),
		buildGrepBlob(),
	}
	for _, in := range inputs {
		req := toolResultReq(in, false)
		Default().Compress(req, Config{Enabled: true})
		require.LessOrEqual(t, len(resultContent(req)), len(in))
	}
}

func TestGrepRule_CapsPerFile(t *testing.T) {
	blob := buildGrepBlob()
	req := toolResultReq(blob, false)
	stats := Default().Compress(req, Config{Enabled: true})
	require.NotNil(t, stats)
	require.Equal(t, "grep", stats.Hits[0].Rule)

	out := resultContent(req)
	require.Contains(t, out, "more matches")
	// No more than grepPerFileMax raw match lines per file should survive.
	count := strings.Count(out, "app.go:")
	require.LessOrEqual(t, count, grepPerFileMax+1) // +1 for the summary line
}

func TestGitDiffRule_PreservesHeadersAndChanges(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/main.go b/main.go\n")
	b.WriteString("@@ -1,200 +1,200 @@\n")
	for i := 0; i < 180; i++ {
		fmt.Fprintf(&b, " unchanged context %d\n", i)
	}
	b.WriteString("+added critical line\n")
	b.WriteString("-removed critical line\n")
	req := toolResultReq(b.String(), false)

	stats := Default().Compress(req, Config{Enabled: true})
	require.NotNil(t, stats)
	out := resultContent(req)
	require.Contains(t, out, "diff --git a/main.go b/main.go")
	require.Contains(t, out, "+added critical line")
	require.Contains(t, out, "-removed critical line")
	require.Contains(t, out, "context lines elided")
}

func TestDedupRule_CollapsesRepeats(t *testing.T) {
	// A long, path-free, colon-free repeated line so no structured rule
	// (ls/find/grep/tree) claims it and dedup deterministically wins.
	line := "this is a fairly long repeated log message that exceeds eighty characters in total length\n"
	blob := strings.Repeat(line, 200)
	req := toolResultReq(blob, false)
	stats := Default().Compress(req, Config{Enabled: true})
	require.NotNil(t, stats)
	require.Equal(t, "dedup-log", stats.Hits[0].Rule)
	require.Less(t, len(resultContent(req)), len(blob))
	require.Contains(t, resultContent(req), "×")
}

func TestEngine_DisabledRuleSkipped(t *testing.T) {
	blob := buildGrepBlob()
	req := toolResultReq(blob, false)
	// Disable grep; some other generic rule may still fire, but not grep.
	stats := Default().Compress(req, Config{Enabled: true, Disabled: []string{"grep"}})
	if stats != nil {
		for _, h := range stats.Hits {
			require.NotEqual(t, "grep", h.Rule)
		}
	}
}

func TestStats_Saved(t *testing.T) {
	s := Stats{BytesBefore: 1000, BytesAfter: 400}
	require.Equal(t, 600, s.Saved())
}

func buildGrepBlob() string {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "app.go:%d: match content here number %d\n", i+1, i)
	}
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "util.go:%d: another match %d\n", i+1, i)
	}
	return b.String()
}

func TestExtendedNoiseDirs(t *testing.T) {
	lines := []string{
		"src", "lib", ".turbo", ".vercel", ".pytest_cache", ".mypy_cache",
		".tox", "env", "coverage", ".nyc_output", "Thumbs.db", ".vs", ".eggs",
		"something.egg-info", "main.go", "readme.md",
	}
	content := strings.Join(lines, "\n")
	// Pad to meet ls detect threshold (needs ≥12 non-empty lines).
	for i := 0; i < 10; i++ {
		content += fmt.Sprintf("\nfile%d.txt", i)
	}
	req := toolResultReq(content, false)
	stats := Default().Compress(req, Config{Enabled: true})
	if stats != nil {
		out := resultContent(req)
		require.Contains(t, out, "noise entries hidden")
		require.NotContains(t, out, ".turbo")
		require.NotContains(t, out, ".vercel")
	}
}

func TestTestRunnerRule_KeepsFailuresOnly(t *testing.T) {
	var b strings.Builder
	b.WriteString("Test Suites: 5 passed, 1 failed, 6 total\n")
	b.WriteString("Tests:       42 passed, 3 failed, 45 total\n")
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, "  ✓ test case %d passes correctly\n", i)
	}
	b.WriteString("  ✗ FAIL: broken test case should fail\n")
	b.WriteString("  ✗ FAIL: another broken test\n")
	req := toolResultReq(b.String(), false)
	stats := Default().Compress(req, Config{Enabled: true})
	require.NotNil(t, stats)
	out := resultContent(req)
	require.Contains(t, out, "FAIL")
	require.Contains(t, out, "passing tests omitted")
}

func TestLintRule_GroupsByFile(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 15; i++ {
		fmt.Fprintf(&b, "src/app.ts:%d:5: error TS2322: Type 'string' not assignable\n", i+1)
	}
	for i := 0; i < 15; i++ {
		fmt.Fprintf(&b, "src/util.ts:%d:3: error TS2345: Argument not assignable\n", i+1)
	}
	req := toolResultReq(b.String(), false)
	stats := Default().Compress(req, Config{Enabled: true, Disabled: []string{"grep", "find"}})
	require.NotNil(t, stats)
	out := resultContent(req)
	require.Contains(t, out, "more issues in")
}

func TestContainerRule_CapsEntries(t *testing.T) {
	var b strings.Builder
	b.WriteString("CONTAINER ID   IMAGE     COMMAND   STATUS    NAMES\n")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&b, "abc%03d        nginx     \"/bin\"    Up %dh    web-%d\n", i, i, i)
	}
	req := toolResultReq(b.String(), false)
	stats := Default().Compress(req, Config{Enabled: true, Disabled: []string{"ls", "find"}})
	require.NotNil(t, stats)
	out := resultContent(req)
	require.Contains(t, out, "CONTAINER ID")
	require.Contains(t, out, "more containers")
}

func TestDiffCompaction_StatSummary(t *testing.T) {
	var b strings.Builder
	for f := 0; f < 5; f++ {
		fmt.Fprintf(&b, "diff --git a/file%d.go b/file%d.go\n", f, f)
		b.WriteString("@@ -1,200 +1,200 @@\n")
		for i := 0; i < 150; i++ {
			fmt.Fprintf(&b, " unchanged line %d in file %d\n", i, f)
		}
		b.WriteString("+added line\n")
		b.WriteString("-removed line\n")
	}
	req := toolResultReq(b.String(), false)
	stats := Default().Compress(req, Config{Enabled: true})
	require.NotNil(t, stats)
	out := resultContent(req)
	require.Contains(t, out, "files changed")
	require.Contains(t, out, "+added line")
}

func TestSourceCodeFilter_MinimalSecondaryPass(t *testing.T) {
	code := "package main\n\nimport \"fmt\"\n\n// This is a long comment that should be stripped\n" +
		"func main() {\n\t// Another comment to remove\n\tfmt.Println(\"hello\")\n}\n" +
		strings.Repeat("// padding comment line to make payload large enough\n", 20)
	req := toolResultReq(code, false)
	stats := Default().Compress(req, Config{Enabled: true, FilterLevel: FilterMinimal})
	if stats != nil {
		out := resultContent(req)
		require.LessOrEqual(t, len(out), len(code))
	}
}