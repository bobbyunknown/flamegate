package slimmer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFilterLevel(t *testing.T) {
	cases := map[string]FilterLevel{
		"none":       FilterNone,
		"minimal":    FilterMinimal,
		"aggressive": FilterAggressive,
		"":           FilterNone,
		"bogus":      FilterNone,
		" Minimal ":  FilterMinimal,
	}
	for input, want := range cases {
		got := ParseFilterLevel(input)
		require.Equal(t, want, got, "ParseFilterLevel(%q)", input)
	}
}

func TestDetectLanguage(t *testing.T) {
	cases := map[string]Language{
		`{"key": "value"}`:                        LangData,
		"---\nname: test\nvalue: 42\n":            LangData,
		"package main\n\nfunc main() {\n}\n":      LangCFamily,
		"def hello():\n    return True\n":         LangPython,
		"#!/bin/bash\necho hello\n":               LangRubyShell,
		"func main() {\n\treturn nil\n}\n":        LangCFamily,
	}
	for input, want := range cases {
		got := detectLanguage(input)
		require.Equal(t, want, got, "detectLanguage(%q)", input[:min(30, len(input))])
	}
}

func TestStripCFamilyComments_Minimal(t *testing.T) {
	input := `package main

// This comment should be removed
func main() {
	// Another comment
	fmt.Println("hello") // inline comment
}
`
	result, err := stripComments(input, FilterMinimal)
	require.NoError(t, err)
	require.NotContains(t, result, "This comment should be removed")
	require.NotContains(t, result, "Another comment")
	require.Contains(t, result, `fmt.Println("hello")`)
	require.Contains(t, result, "func main()")
}

func TestStripCFamilyComments_BlockComment(t *testing.T) {
	input := `package main

/*
 * This is a block comment
 * spanning multiple lines
 */
func main() {
	x := 1 /* inline block */ + 2
}
`
	result, err := stripComments(input, FilterMinimal)
	require.NoError(t, err)
	require.NotContains(t, result, "block comment")
	require.Contains(t, result, "func main()")
}

func TestStripPythonComments_Minimal(t *testing.T) {
	input := `import os

# This comment should go
def hello():
    # Another comment
    return True  # inline comment
`
	result, err := stripComments(input, FilterMinimal)
	require.NoError(t, err)
	require.NotContains(t, result, "This comment should go")
	require.NotContains(t, result, "Another comment")
	require.Contains(t, result, "def hello()")
	require.Contains(t, result, "return True")
}

func TestStripPythonComments_AggressiveDocstrings(t *testing.T) {
	input := `import os

def hello():
    """This docstring should be removed in aggressive mode."""
    return True

class Foo:
    """
    Multi-line docstring
    that should also go.
    """
    pass
`
	result, err := stripComments(input, FilterAggressive)
	require.NoError(t, err)
	require.NotContains(t, result, "docstring should be removed")
	require.NotContains(t, result, "Multi-line docstring")
	require.Contains(t, result, "def hello()")
	require.Contains(t, result, "return True")
}

func TestStripHashComments(t *testing.T) {
	input := `#!/bin/bash

# This is a comment
echo "hello" # inline
ls -la
`
	result, err := stripComments(input, FilterMinimal)
	require.NoError(t, err)
	require.Contains(t, result, "#!/bin/bash")
	require.NotContains(t, result, "This is a comment")
	require.Contains(t, result, `echo "hello"`)
}

func TestStripAggressive_BlankLines(t *testing.T) {
	input := `package main


import "fmt"



func main() {
	fmt.Println("hi")
}
`
	result := stripBlankLinesAndTrailing(input)
	lines := strings.Split(result, "\n")
	consecutiveBlanks := 0
	for _, l := range lines {
		if l == "" {
			consecutiveBlanks++
			require.LessOrEqual(t, consecutiveBlanks, 1, "no consecutive blank lines allowed")
		} else {
			consecutiveBlanks = 0
		}
	}
}

func TestSourceCodeRule_Detect(t *testing.T) {
	r := sourceCodeRule{}

	code := "package main\n\n// comment here\nfunc main() {\n\treturn nil\n}\n"
	require.Greater(t, r.Detect(code), 0.0, "should detect Go source")

	data := `{"key": "value", "nested": {"a": 1}}`
	require.Equal(t, 0.0, r.Detect(data), "should not detect JSON data")
}

func TestStripComments_NoGrowSafety(t *testing.T) {
	tiny := "x := 1\n"
	result, err := stripComments(tiny, FilterMinimal)
	require.NoError(t, err)
	require.LessOrEqual(t, len(result), len(tiny))
}
