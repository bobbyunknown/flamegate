// Package ponytail implements FlameGate's output-token saving mode by injecting
// a leveled "lazy senior developer" system-prompt block that biases the model
// toward minimal code.
//
// It mirrors the terse package's structure (an HTML-comment sentinel for
// idempotency across retries and chained fallbacks) but APPENDS its block after
// any existing system text rather than prepending. This keeps Terse/Caveman
// directives intact while still steering output style.
//
// This is a request-side transform: it only modifies the system prompt and runs
// before format translation so it applies uniformly across every provider
// dialect.
package ponytail

import (
	"strings"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// sentinel marks instruction text FlameGate injected, so Apply is idempotent
// across retries and chained fallbacks. It is an HTML-style comment that models
// ignore in their output.
const sentinel = "<!-- flamegate:ponytail -->"

// Level selects the intensity of the injected Ponytail prompt.
type Level string

const (
	// LevelLite builds what's asked but names the lazier alternative.
	LevelLite Level = "lite"
	// LevelFull (default) enforces the laziness ladder.
	LevelFull Level = "full"
	// LevelUltra is the YAGNI-extremist intensity.
	LevelUltra Level = "ultra"
)

// Config controls Ponytail behavior for a request.
type Config struct {
	Enabled bool
	Level   Level
}

// ValidLevel reports whether s is one of lite/full/ultra (case-sensitive).
func ValidLevel(s Level) bool {
	switch s {
	case LevelLite, LevelFull, LevelUltra:
		return true
	default:
		return false
	}
}

// Apply appends the leveled "lazy senior developer" prompt block (carrying the
// sentinel) after any existing system text in req.System when cfg.Enabled.
//
// It is a no-op when disabled or when the sentinel is already present, so it is
// safe to call repeatedly across retries and fallback attempts. An invalid
// level falls back to LevelFull. When the system text is empty or whitespace,
// req.System is set to the block; otherwise the block is appended after the
// existing text so Terse/Caveman directives are preserved.
func Apply(req *core.ChatRequest, cfg Config) {
	if req == nil || !cfg.Enabled {
		return
	}
	if strings.Contains(req.System, sentinel) {
		return
	}

	level := cfg.Level
	if !ValidLevel(level) {
		level = LevelFull
	}

	block := sentinel + "\n" + prompts[level]
	if strings.TrimSpace(req.System) == "" {
		req.System = block
		return
	}
	req.System = req.System + "\n\n" + block
}
