package slimmer

import (
	"regexp"
	"strings"
)

// FilterLevel selects how aggressively source code comments are stripped.
type FilterLevel int

const (
	// FilterNone disables source code filtering (default, backward compatible).
	FilterNone FilterLevel = iota
	// FilterMinimal strips line and block comments only.
	FilterMinimal
	// FilterAggressive strips comments, docstrings, blank lines, and trailing
	// whitespace.
	FilterAggressive
)

// ParseFilterLevel converts a settings string to a FilterLevel. Unrecognized
// values default to FilterNone for safety.
func ParseFilterLevel(s string) FilterLevel {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "minimal":
		return FilterMinimal
	case "aggressive":
		return FilterAggressive
	default:
		return FilterNone
	}
}

// Language identifies a programming language detected from tool-result content.
type Language int

const (
	LangUnknown Language = iota
	LangCFamily          // Go, JS, TS, Rust, Java, C, C++
	LangPython
	LangRubyShell // Ruby, Shell — #-style comments only
	LangData      // JSON, YAML, TOML, XML — no comment stripping
)

// detectLanguage guesses the language from syntax patterns in the content probe.
// Returns LangData when the content looks like a data format (no stripping).
func detectLanguage(content string) Language {
	probe := content
	if len(probe) > 2048 {
		probe = probe[:2048]
	}

	// Data formats — never strip comments.
	if looksLikeData(probe) {
		return LangData
	}

	// Python indicators.
	if strings.Contains(probe, "def ") || strings.Contains(probe, "import ") ||
		strings.Contains(probe, "class ") && strings.Contains(probe, ":") ||
		strings.Contains(probe, `"""`) {
		return LangPython
	}

	// Ruby/Shell indicators (must check before C-family since # comments overlap).
	if strings.Contains(probe, "require ") && strings.Contains(probe, "end\n") {
		return LangRubyShell
	}
	if strings.HasPrefix(probe, "#!/bin/") || strings.HasPrefix(probe, "#!/usr/bin/") ||
		strings.Contains(probe, "then\n") || strings.Contains(probe, "fi\n") {
		return LangRubyShell
	}

	// C-family indicators.
	if strings.Contains(probe, "func ") || strings.Contains(probe, "fn ") ||
		strings.Contains(probe, "package ") || strings.Contains(probe, "import ") ||
		strings.Contains(probe, "class ") || strings.Contains(probe, "interface ") ||
		strings.Contains(probe, "function ") || strings.Contains(probe, "const ") ||
		strings.Contains(probe, "let ") || strings.Contains(probe, "var ") {
		return LangCFamily
	}

	return LangUnknown
}

var reDataFormat = regexp.MustCompile(`(?m)^\s*("[\w-]+"\s*:|[<{[]\s*$|\w+\s*=\s*["{[]|---\s*$)`)

func looksLikeData(probe string) bool {
	// JSON/YAML/TOML indicators.
	trimmed := strings.TrimSpace(probe)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return true
	}
	if strings.Contains(trimmed, "---\n") { // YAML document separator
		return true
	}
	matches := reDataFormat.FindAllString(probe, -1)
	return len(matches) > len(strings.Split(probe, "\n"))/4
}

// sourceCodeRule strips comments from source code in tool results.
// It only fires when no other rule matches (low detect score) and the content
// looks like source code.
type sourceCodeRule struct{}

func (sourceCodeRule) Name() string { return "source-code" }

func (sourceCodeRule) Detect(probe string) float64 {
	if looksLikeData(probe) {
		return 0
	}

	// Look for comment syntax and code keywords.
	hasComments := strings.Contains(probe, "//") || strings.Contains(probe, "/*") ||
		strings.Contains(probe, "# ")
	hasCode := strings.Contains(probe, "func ") || strings.Contains(probe, "def ") ||
		strings.Contains(probe, "fn ") || strings.Contains(probe, "class ") ||
		strings.Contains(probe, "import ") || strings.Contains(probe, "package ") ||
		strings.Contains(probe, "return ") || strings.Contains(probe, "if ")

	if hasComments && hasCode {
		return 0.55
	}
	return 0
}

// Compress strips comments from source code content. The filter level is
// determined by the engine's Config; this method uses FilterMinimal as the
// default when called directly. The engine applies aggressive filtering as a
// secondary pass when configured.
func (sourceCodeRule) Compress(content string) (string, error) {
	return stripComments(content, FilterMinimal)
}

// stripComments removes comments from content based on detected language and
// filter level. Returns original content when the language is LangData or
// when stripping would not reduce size.
func stripComments(content string, level FilterLevel) (string, error) {
	if level == FilterNone {
		return content, nil
	}

	lang := detectLanguage(content)
	if lang == LangData {
		return content, nil
	}

	var result string
	switch lang {
	case LangCFamily:
		result = stripCFamilyComments(content, level)
	case LangPython:
		result = stripPythonComments(content, level)
	case LangRubyShell:
		result = stripHashComments(content, level)
	default:
		result = stripCFamilyComments(content, level) // conservative default
	}

	if level == FilterAggressive {
		result = stripBlankLinesAndTrailing(result)
	}

	// Safety: never return larger content.
	if len(result) >= len(content) || result == "" {
		return content, nil
	}
	return result, nil
}

// stripCFamilyComments removes // line comments and /* */ block comments.
func stripCFamilyComments(content string, level FilterLevel) string {
	var out []string
	inBlock := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)

		if inBlock {
			if idx := strings.Index(line, "*/"); idx >= 0 {
				inBlock = false
				// Keep the part after */ if any.
				rest := strings.TrimSpace(line[idx+2:])
				if rest != "" {
					out = append(out, rest)
				}
			}
			continue
		}

		if strings.Contains(trimmed, "/*") {
			if idx := strings.Index(line, "/*"); idx >= 0 {
				// Check if block comment closes on same line.
				if closeIdx := strings.Index(line[idx+2:], "*/"); closeIdx >= 0 {
					// Inline block comment: keep code before and after.
					before := line[:idx]
					after := strings.TrimSpace(line[idx+2+closeIdx+2:])
					combined := strings.TrimSpace(before + " " + after)
					if combined != "" {
						out = append(out, combined)
					} else if level != FilterAggressive {
						out = append(out, line) // preserve original for minimal
					}
					continue
				}
				// Multi-line block comment starts.
				before := strings.TrimSpace(line[:idx])
				if before != "" {
					out = append(out, before)
				}
				inBlock = true
				continue
			}
		}

		// Line comment: strip if it's a pure comment line.
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Inline comment: strip the // part but keep code.
		if idx := findInlineComment(line, "//"); idx >= 0 {
			out = append(out, strings.TrimRight(line[:idx], " \t"))
			continue
		}

		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// stripPythonComments removes # line comments and optionally triple-quote docstrings.
func stripPythonComments(content string, level FilterLevel) string {
	var out []string
	inDocstring := false
	docChar := ""

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)

		if inDocstring {
			if strings.Contains(trimmed, docChar) {
				inDocstring = false
			}
			continue
		}

		if level == FilterAggressive {
			for _, q := range []string{`"""`, `'''`} {
				if strings.HasPrefix(trimmed, q) {
					// Check if docstring closes on same line.
					rest := trimmed[3:]
					if strings.Contains(rest, q) {
						// Single-line docstring: skip.
						goto nextLine
					}
					inDocstring = true
					docChar = q
					goto nextLine
				}
			}
		}

		// Pure # comment line.
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Inline # comment.
		if idx := findInlineComment(line, "#"); idx >= 0 {
			out = append(out, strings.TrimRight(line[:idx], " \t"))
			continue
		}

		out = append(out, line)
	nextLine:
	}
	return strings.Join(out, "\n")
}

// stripHashComments removes #-style comments (Ruby, Shell).
func stripHashComments(content string, level FilterLevel) string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		// Shebangs and pure comment lines.
		if strings.HasPrefix(trimmed, "#!") {
			out = append(out, line) // preserve shebangs
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Inline # comment.
		if idx := findInlineComment(line, "#"); idx >= 0 {
			out = append(out, strings.TrimRight(line[:idx], " \t"))
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// findInlineComment locates a comment marker that isn't inside a string
// literal. This is a heuristic: it checks that the marker is preceded by
// whitespace and not inside quotes. Good enough for compression purposes.
func findInlineComment(line, marker string) int {
	idx := strings.Index(line, " "+marker)
	if idx < 0 {
		idx = strings.Index(line, "\t"+marker)
	}
	if idx < 0 {
		return -1
	}
	// Skip if inside quotes (simple heuristic: count quotes before marker).
	before := line[:idx]
	singles := strings.Count(before, "'")
	doubles := strings.Count(before, "\"")
	if singles%2 != 0 || doubles%2 != 0 {
		return -1 // likely inside a string
	}
	return idx + 1 // skip the leading space/tab
}

// stripBlankLinesAndTrailing removes consecutive blank lines (keeping at most
// one) and trims trailing whitespace from each line.
func stripBlankLinesAndTrailing(content string) string {
	var out []string
	prevBlank := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimRight(line, " \t")
		blank := trimmed == ""
		if blank && prevBlank {
			continue
		}
		out = append(out, trimmed)
		prevBlank = blank
	}
	return strings.Join(out, "\n")
}
