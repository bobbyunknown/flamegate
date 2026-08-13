package slimmer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// BuiltinRules returns the default rule set in priority order. The engine picks
// by Detect score, but ordering documents intent and breaks exact-score ties in
// favor of the more specific rule.
func BuiltinRules() []Rule {
	return []Rule{
		gitDiffRule{},
		gitStatusRule{},
		buildLogRule{},
		grepRule{},
		findRule{},
		treeRule{},
		lsRule{},
		numberedReadRule{},
		smartDedupLogRule{},
		testRunnerRule{},
		lintRule{},
		containerRule{},
		truncateRule{},
		sourceCodeRule{},
	}
}

// Tuning thresholds. Kept as named constants so behavior is auditable.
const (
	diffHunkMaxLines    = 100
	diffContextKeep     = 3
	diffCompactMaxLines = 500
	grepPerFileMax      = 10
	findPerDirMax       = 10
	treeMaxLines        = 200
	dedupMinLines       = 5
	truncateHeadLines   = 60
	truncateTailLines   = 40
	truncateTriggerCap  = 240 // only truncate inputs longer than this many lines
)

// ---- git-diff ---------------------------------------------------------------

var (
	reDiffHeader = regexp.MustCompile(`(?m)^diff --git `)
	reHunk       = regexp.MustCompile(`^@@ .* @@`)
)

type gitDiffRule struct{}

func (gitDiffRule) Name() string { return "git-diff" }

func (gitDiffRule) Detect(probe string) float64 {
	if reDiffHeader.MatchString(probe) {
		return 1.0
	}
	if strings.Contains(probe, "\n@@ ") || strings.HasPrefix(probe, "@@ ") {
		return 0.8
	}
	return 0
}

// Compress trims oversized hunks while preserving every changed line and the
// surrounding context up to diffContextKeep lines. File and hunk headers are
// always kept so the model retains structure. For large diffs, a stat summary
// is prepended.
func (gitDiffRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	var out []string

	// Collect stat info per-file before compression.
	statSummary := buildDiffStatSummary(lines)

	flushHunk := func(buf []string) {
		if len(buf) <= diffHunkMaxLines {
			out = append(out, buf...)
			return
		}
		// Large hunk: keep changed lines plus limited context, elide long runs
		// of unchanged context.
		var kept []string
		ctx := 0
		for _, l := range buf {
			changed := strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-")
			switch {
			case changed:
				kept = append(kept, l)
				ctx = 0
			case ctx < diffContextKeep:
				kept = append(kept, l)
				ctx++
			default:
				// skip surplus context
			}
		}
		removed := len(buf) - len(kept)
		kept = append(kept, fmt.Sprintf("… %d context lines elided", removed))
		out = append(out, kept...)
	}

	var hunk []string
	inHunk := false
	for _, l := range lines {
		if reHunk.MatchString(l) {
			if inHunk {
				flushHunk(hunk)
			}
			hunk = []string{l}
			inHunk = true
			continue
		}
		if inHunk {
			hunk = append(hunk, l)
		} else {
			out = append(out, l)
		}
	}
	if inHunk {
		flushHunk(hunk)
	}

	// If compressed output is large, prepend stat summary.
	if len(out) > diffCompactMaxLines && statSummary != "" {
		// Keep stat + first diffCompactMaxLines lines of detail.
		truncated := out
		if len(truncated) > diffCompactMaxLines {
			truncated = append(out[:diffCompactMaxLines],
				fmt.Sprintf("… %d diff lines truncated (see stat summary above)", len(out)-diffCompactMaxLines))
		}
		out = append([]string{statSummary, ""}, truncated...)
	} else if statSummary != "" && len(lines) > 100 {
		// For moderately large diffs, prepend stat for context.
		out = append([]string{statSummary, ""}, out...)
	}

	return strings.Join(out, "\n"), nil
}

// buildDiffStatSummary generates a stat-style summary from diff lines,
// counting added (+) and removed (-) lines per file.
func buildDiffStatSummary(lines []string) string {
	type fileStat struct {
		added   int
		removed int
	}
	files := make(map[string]*fileStat)
	var order []string
	currentFile := ""

	for _, l := range lines {
		if strings.HasPrefix(l, "diff --git ") {
			// Extract filename from "diff --git a/path b/path"
			parts := strings.SplitN(l, " b/", 2)
			if len(parts) == 2 {
				currentFile = parts[1]
				if _, exists := files[currentFile]; !exists {
					files[currentFile] = &fileStat{}
					order = append(order, currentFile)
				}
			}
			continue
		}
		if currentFile == "" {
			continue
		}
		fs := files[currentFile]
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			fs.added++
		} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
			fs.removed++
		}
	}

	if len(order) == 0 {
		return ""
	}

	var out []string
	totalAdd, totalDel := 0, 0
	for _, f := range order {
		fs := files[f]
		totalAdd += fs.added
		totalDel += fs.removed
		out = append(out, fmt.Sprintf(" %s | +%d -%d", f, fs.added, fs.removed))
	}
	out = append(out, fmt.Sprintf(" %d files changed, +%d -%d",
		len(order), totalAdd, totalDel))
	return strings.Join(out, "\n")
}

// ---- git-status -------------------------------------------------------------

var reStatusEntry = regexp.MustCompile(`(?m)^\s*(modified|new file|deleted|renamed|untracked|added|copied|typechange):`)

type gitStatusRule struct{}

func (gitStatusRule) Name() string { return "git-status" }

func (gitStatusRule) Detect(probe string) float64 {
	score := 0.0
	if strings.Contains(probe, "Changes not staged for commit") ||
		strings.Contains(probe, "Changes to be committed") ||
		strings.Contains(probe, "Untracked files:") {
		score += 0.6
	}
	if reStatusEntry.MatchString(probe) {
		score += 0.3
	}
	if score > 1 {
		score = 1
	}
	return score
}

// Compress caps long file listings, replacing the overflow with a count.
func (gitStatusRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	var out []string
	kept := 0
	overflow := 0
	for _, l := range lines {
		if reStatusEntry.MatchString(l) {
			if kept < CapList {
				out = append(out, l)
				kept++
			} else {
				overflow++
			}
			continue
		}
		if overflow > 0 {
			out = append(out, fmt.Sprintf("\t… and %d more files", overflow))
			overflow = 0
		}
		out = append(out, l)
	}
	if overflow > 0 {
		out = append(out, fmt.Sprintf("\t… and %d more files", overflow))
	}
	return strings.Join(out, "\n"), nil
}

// ---- grep -------------------------------------------------------------------

// reGrepLine matches "path:line:content" or "path:content" grep output.
var reGrepLine = regexp.MustCompile(`^([^:\n]+):(\d+):`)

type grepRule struct{}

func (grepRule) Name() string { return "grep" }

func (grepRule) Detect(probe string) float64 {
	lines := nonEmptyLines(probe)
	if len(lines) < 2 {
		return 0
	}
	matches := 0
	for _, l := range lines {
		if reGrepLine.MatchString(l) {
			matches++
		}
	}
	ratio := float64(matches) / float64(len(lines))
	if ratio >= 0.8 {
		return 0.9
	}
	if ratio >= 0.5 {
		return 0.6
	}
	return 0
}

// Compress caps the number of matches shown per file.
func (grepRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	perFile := map[string]int{}
	var out []string
	suppressed := map[string]int{}

	for _, l := range lines {
		m := reGrepLine.FindStringSubmatch(l)
		if m == nil {
			out = append(out, l)
			continue
		}
		file := m[1]
		perFile[file]++
		if perFile[file] <= grepPerFileMax {
			out = append(out, l)
		} else {
			suppressed[file]++
		}
	}
	if len(suppressed) > 0 {
		files := sortedKeys(suppressed)
		for _, f := range files {
			out = append(out, fmt.Sprintf("%s: … %d more matches", f, suppressed[f]))
		}
	}
	return strings.Join(out, "\n"), nil
}

// ---- find -------------------------------------------------------------------

type findRule struct{}

func (findRule) Name() string { return "find" }

func (findRule) Detect(probe string) float64 {
	lines := nonEmptyLines(probe)
	if len(lines) < 8 {
		return 0
	}
	pathish := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "./") || strings.HasPrefix(l, "/") || strings.Contains(l, "/") {
			pathish++
		}
	}
	if float64(pathish)/float64(len(lines)) >= 0.9 {
		return 0.7
	}
	return 0
}

// Compress groups entries by parent directory and caps entries per directory.
func (findRule) Compress(content string) (string, error) {
	lines := nonEmptyLines(content)
	perDir := map[string]int{}
	var out []string
	overflow := map[string]int{}

	for _, l := range lines {
		dir := parentDir(l)
		perDir[dir]++
		if perDir[dir] <= findPerDirMax {
			out = append(out, l)
		} else {
			overflow[dir]++
		}
	}
	for _, d := range sortedKeys(overflow) {
		out = append(out, fmt.Sprintf("%s/… %d more entries", strings.TrimRight(d, "/"), overflow[d]))
	}
	return strings.Join(out, "\n"), nil
}

// ---- tree -------------------------------------------------------------------

type treeRule struct{}

func (treeRule) Name() string { return "tree" }

func (treeRule) Detect(probe string) float64 {
	if strings.Contains(probe, "├──") || strings.Contains(probe, "└──") || strings.Contains(probe, "│  ") {
		return 0.85
	}
	return 0
}

// Compress caps the number of tree lines, keeping the head and noting the rest.
func (treeRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) <= treeMaxLines {
		return content, nil
	}
	head := lines[:treeMaxLines]
	out := append([]string{}, head...)
	out = append(out, fmt.Sprintf("… %d more tree entries", len(lines)-treeMaxLines))
	return strings.Join(out, "\n"), nil
}

// ---- ls ---------------------------------------------------------------------

var lsNoiseDirs = map[string]struct{}{
	"node_modules": {}, ".git": {}, "target": {}, "__pycache__": {},
	".next": {}, "dist": {}, "build": {}, ".venv": {}, "venv": {},
	".cache": {}, ".idea": {}, ".vscode": {}, ".DS_Store": {},
	".turbo": {}, ".vercel": {}, ".pytest_cache": {}, ".mypy_cache": {},
	".tox": {}, "env": {}, "coverage": {}, ".nyc_output": {},
	"Thumbs.db": {}, ".vs": {}, ".eggs": {},
}

type lsRule struct{}

func (lsRule) Name() string { return "ls" }

func (lsRule) Detect(probe string) float64 {
	lines := nonEmptyLines(probe)
	if len(lines) < 12 {
		return 0
	}
	// Flat list of short names without path separators reads like `ls`.
	flat := 0
	for _, l := range lines {
		if !strings.Contains(l, "/") && !strings.Contains(l, ":") && len(l) < 80 {
			flat++
		}
	}
	if float64(flat)/float64(len(lines)) >= 0.85 {
		return 0.55
	}
	return 0
}

// Compress drops well-known noise directories and summarizes files by extension
// when the listing is long.
func (lsRule) Compress(content string) (string, error) {
	lines := nonEmptyLines(content)
	var kept []string
	extCount := map[string]int{}
	dropped := 0

	for _, l := range lines {
		name := strings.TrimSpace(l)
		if _, noise := lsNoiseDirs[name]; noise || strings.HasSuffix(name, ".egg-info") {
			dropped++
			continue
		}
		kept = append(kept, l)
		if dot := strings.LastIndex(name, "."); dot > 0 {
			extCount["."+name[dot+1:]]++
		}
	}

	var b strings.Builder
	b.WriteString(strings.Join(kept, "\n"))
	if dropped > 0 {
		fmt.Fprintf(&b, "\n(%d noise entries hidden)", dropped)
	}
	if len(extCount) > 1 {
		b.WriteString("\nby type: " + topExtensions(extCount, 5))
	}
	return b.String(), nil
}

// ---- numbered file reads ----------------------------------------------------

var reNumbered = regexp.MustCompile(`^\s*\d+\s*[|:\t]`)

type numberedReadRule struct{}

func (numberedReadRule) Name() string { return "numbered-read" }

func (numberedReadRule) Detect(probe string) float64 {
	lines := nonEmptyLines(probe)
	if len(lines) < 5 {
		return 0
	}
	numbered := 0
	for _, l := range lines {
		if reNumbered.MatchString(l) {
			numbered++
		}
	}
	if float64(numbered)/float64(len(lines)) >= 0.9 {
		return 0.6
	}
	return 0
}

// Compress applies head/tail truncation to very long numbered reads, preserving
// line numbers so references stay valid.
func (numberedReadRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) <= truncateTriggerCap {
		return content, nil
	}
	return headTailElide(lines, truncateHeadLines, truncateTailLines), nil
}

// ---- build-output -----------------------------------------------------------

var (
	reBuildNoise    = regexp.MustCompile(`(?i)^\s*(\[\d+/\d+\]|Compiling |Downloading |downloaded |Fetching |npm WARN |info |warning: unused|Progress|\d+%\s)`)
	reBuildSignal   = regexp.MustCompile(`(?i)(error|failed|panic|exception|warning|cannot find|undefined)`)
	reBuildProgress = regexp.MustCompile(`(?m)^\s*\[\d+/\d+\]`)
)

type buildLogRule struct{}

func (buildLogRule) Name() string { return "build-output" }

func (buildLogRule) Detect(probe string) float64 {
	if strings.Contains(probe, "Compiling ") || strings.Contains(probe, "npm WARN") ||
		strings.Contains(probe, "webpack") || strings.Contains(probe, "tsc ") ||
		reBuildProgress.MatchString(probe) {
		return 0.65
	}
	return 0
}

// Compress drops progress/noise lines but always keeps lines that look like
// errors or warnings.
func (buildLogRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	var out []string
	dropped := 0
	for _, l := range lines {
		if reBuildSignal.MatchString(l) {
			out = append(out, l)
			continue
		}
		if reBuildNoise.MatchString(l) {
			dropped++
			continue
		}
		out = append(out, l)
	}
	if dropped > 0 {
		out = append(out, fmt.Sprintf("(%d progress/noise lines removed)", dropped))
	}
	return strings.Join(out, "\n"), nil
}

// ---- smart truncate (generic fallback) -------------------------------------

type truncateRule struct{}

func (truncateRule) Name() string { return "truncate" }

func (truncateRule) Detect(probe string) float64 {
	// Lowest-priority generic fallback for very large blobs of any shape.
	if len(nonEmptyLines(probe)) >= dedupMinLines {
		return 0.45 // just below threshold; only fires if nothing else matched
	}
	return 0
}

func (truncateRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) <= truncateTriggerCap {
		return content, nil
	}
	return headTailElide(lines, truncateHeadLines, truncateTailLines), nil
}

// ---- test-runner output -----------------------------------------------------

var (
	reTestFail    = regexp.MustCompile(`(?i)(FAIL|FAILED|❌|✗|✘)`)
	reTestPass    = regexp.MustCompile(`(?i)(PASS|PASSED|✓|✔|ok\s)`)
	reTestSummary = regexp.MustCompile(`(?i)(Tests?:|Test Suites?:|test result:|FAILED|failed)`)
)

type testRunnerRule struct{}

func (testRunnerRule) Name() string { return "test-runner" }

func (testRunnerRule) Detect(probe string) float64 {
	if strings.Contains(probe, "test result:") ||
		strings.Contains(probe, "Test Suites:") ||
		strings.Contains(probe, "Tests: ") ||
		(reTestFail.MatchString(probe) && reTestPass.MatchString(probe)) {
		return 0.75
	}
	return 0
}

// Compress keeps only failures and summary lines, dropping passing tests.
func (testRunnerRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	var failures, summary, passing []string

	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}

		// Always keep summary lines.
		if reTestSummary.MatchString(trimmed) {
			summary = append(summary, l)
			continue
		}

		// Keep failures, drop passes.
		if reTestFail.MatchString(trimmed) && !reTestPass.MatchString(trimmed) {
			failures = append(failures, l)
		} else if reTestPass.MatchString(trimmed) {
			passing = append(passing, trimmed)
		} else {
			// Context lines — keep if near a failure.
			failures = append(failures, l)
		}
	}

	var out []string
	if len(summary) > 0 {
		out = append(out, summary...)
		out = append(out, "")
	}
	if len(failures) > 0 {
		if len(failures) > CapErrors {
			out = append(out, failures[:CapErrors]...)
			out = append(out, fmt.Sprintf("… %d more failure lines", len(failures)-CapErrors))
		} else {
			out = append(out, failures...)
		}
	}
	if len(passing) > 0 {
		out = append(out, fmt.Sprintf("(%d passing tests omitted)", len(passing)))
	}

	result := strings.Join(out, "\n")
	if len(result) >= len(content) || result == "" {
		return content, nil
	}
	return result, nil
}

// ---- lint / compiler output ------------------------------------------------

var reLintError = regexp.MustCompile(`(?i)(error\s*(TS|E\[|\[)|error:|✖|✗|error\s+\d)`)

type lintRule struct{}

func (lintRule) Name() string { return "lint-output" }

func (lintRule) Detect(probe string) float64 {
	if reLintError.MatchString(probe) ||
		strings.Contains(probe, "warning:") ||
		strings.Contains(probe, "error TS") {
		return 0.7
	}
	return 0
}

// Compress groups errors by file and caps per-file entries.
func (lintRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	perFile := map[string][]string{}
	var order []string
	var headerLines []string

	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" {
			continue
		}

		// Detect file:line:col pattern to group by file.
		file := extractLintFile(trimmed)
		if file != "" {
			if _, seen := perFile[file]; !seen {
				order = append(order, file)
			}
			perFile[file] = append(perFile[file], l)
		} else if !reLintError.MatchString(trimmed) && !strings.Contains(trimmed, "warning:") {
			// Non-error/warning line — likely header or summary.
			headerLines = append(headerLines, l)
		} else {
			// Error/warning without file context — keep as-is.
			headerLines = append(headerLines, l)
		}
	}

	var out []string
	out = append(out, headerLines...)

	for _, file := range order {
		entries := perFile[file]
		if len(entries) > CapWarnings {
			out = append(out, entries[:CapWarnings]...)
			out = append(out, fmt.Sprintf("  … %d more issues in %s", len(entries)-CapWarnings, file))
		} else {
			out = append(out, entries...)
		}
	}

	result := strings.Join(out, "\n")
	if len(result) >= len(content) || result == "" {
		return content, nil
	}
	return result, nil
}

// extractLintFile extracts a filename from a lint/compiler error line.
// Matches patterns like "path/to/file.ext:10:5:" or "path/to/file.ext(10,5):".
var reLintFile = regexp.MustCompile(`^([^\s:]+\.\w+)[:\(]`)

func extractLintFile(line string) string {
	m := reLintFile.FindStringSubmatch(line)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// ---- container list (docker ps, kubectl get pods) --------------------------

var reContainerHeader = regexp.MustCompile(`(?i)^CONTAINER ID|^NAME\s+READY|^NAME\s+STATUS`)

type containerRule struct{}

func (containerRule) Name() string { return "container-list" }

func (containerRule) Detect(probe string) float64 {
	if reContainerHeader.MatchString(probe) {
		return 0.7
	}
	return 0
}

// Compress caps container list entries while keeping the header.
func (containerRule) Compress(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) <= CapList+1 {
		return content, nil
	}

	// Keep header + capped entries.
	header := lines[0]
	entries := lines[1:]
	nonEmpty := nonEmptyLines(strings.Join(entries, "\n"))

	if len(nonEmpty) <= CapList {
		return content, nil
	}

	var out []string
	out = append(out, header)
	out = append(out, nonEmpty[:CapList]...)
	out = append(out, fmt.Sprintf("… %d more containers", len(nonEmpty)-CapList))

	result := strings.Join(out, "\n")
	if len(result) >= len(content) {
		return content, nil
	}
	return result, nil
}

// ---- shared helpers ---------------------------------------------------------

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func parentDir(path string) string {
	path = strings.TrimPrefix(path, "./")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i]
	}
	return "."
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func topExtensions(counts map[string]int, n int) string {
	type kv struct {
		ext string
		n   int
	}
	pairs := make([]kv, 0, len(counts))
	for e, c := range counts {
		pairs = append(pairs, kv{e, c})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].n != pairs[j].n {
			return pairs[i].n > pairs[j].n
		}
		return pairs[i].ext < pairs[j].ext
	})
	if len(pairs) > n {
		pairs = pairs[:n]
	}
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = fmt.Sprintf("%s×%d", p.ext, p.n)
	}
	return strings.Join(parts, ", ")
}

// headTailElide keeps the first head and last tail lines, replacing the middle
// with a single elision marker carrying the omitted line count.
func headTailElide(lines []string, head, tail int) string {
	if len(lines) <= head+tail {
		return strings.Join(lines, "\n")
	}
	omitted := len(lines) - head - tail
	out := make([]string, 0, head+tail+1)
	out = append(out, lines[:head]...)
	out = append(out, fmt.Sprintf("… %d lines elided …", omitted))
	out = append(out, lines[len(lines)-tail:]...)
	return strings.Join(out, "\n")
}
