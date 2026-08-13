package caveman

import (
	"strings"
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func TestApplyDisabled(t *testing.T) {
	req := &core.ChatRequest{System: "original"}
	Apply(req, Config{Enabled: false})
	if req.System != "original" {
		t.Fatalf("disabled caveman must not change system, got %q", req.System)
	}
}

func TestApplyInjectsIntoEmptySystem(t *testing.T) {
	req := &core.ChatRequest{}
	Apply(req, Config{Enabled: true, Level: LevelFull})
	if !strings.Contains(req.System, idempotencyProbe) {
		t.Fatal("expected style directive in system prompt")
	}
	if !strings.Contains(req.System, "terse, information-dense") {
		t.Fatalf("expected full-level prompt, got %q", req.System)
	}
}

// The injected guideline must not carry any visible marker comment or name
// itself, so agentic coding tools treat it as part of the operator's own
// system prompt rather than rejecting it as injected content.
func TestApplyAddsNoVisibleMarkerOrSelfName(t *testing.T) {
	req := &core.ChatRequest{}
	Apply(req, Config{Enabled: true, Level: LevelFull})
	for _, banned := range []string{"<!--", "flamegate:", "caveman mode", "smart caveman"} {
		if strings.Contains(req.System, banned) {
			t.Errorf("system prompt must not contain %q (reads as injected content), got %q", banned, req.System)
		}
	}
}

func TestApplyAppendsToExistingSystem(t *testing.T) {
	req := &core.ChatRequest{System: "be helpful"}
	Apply(req, Config{Enabled: true, Level: LevelLite})
	if !strings.HasPrefix(req.System, "be helpful") {
		t.Fatalf("existing system text must be preserved first, got %q", req.System)
	}
	if !strings.Contains(req.System, idempotencyProbe) {
		t.Fatal("expected style directive appended")
	}
}

func TestApplyIdempotent(t *testing.T) {
	req := &core.ChatRequest{}
	Apply(req, Config{Enabled: true, Level: LevelFull})
	first := req.System
	Apply(req, Config{Enabled: true, Level: LevelFull})
	if req.System != first {
		t.Fatal("second Apply must be a no-op (idempotent across retries)")
	}
}

func TestLevelSelection(t *testing.T) {
	cases := map[Level]string{
		LevelLite:  "Keep grammar",
		LevelFull:  "terse, information-dense",
		LevelUltra: "ultra-terse",
	}
	for level, want := range cases {
		req := &core.ChatRequest{}
		Apply(req, Config{Enabled: true, Level: level})
		if !strings.Contains(req.System, want) {
			t.Errorf("level %q: expected %q in prompt, got %q", level, want, req.System)
		}
	}
}

func TestValidLevel(t *testing.T) {
	for _, l := range []Level{LevelLite, LevelFull, LevelUltra,
		LevelWenyanLite, LevelWenyanFull, LevelWenyanUltra} {
		if !ValidLevel(l) {
			t.Errorf("expected %q to be valid", l)
		}
	}
	if ValidLevel("bogus") {
		t.Error("expected bogus level to be invalid")
	}
}

func TestWenyanLevelSelection(t *testing.T) {
	cases := map[Level]string{
		LevelWenyanLite:  "semi-classical",
		LevelWenyanFull:  "文言文",
		LevelWenyanUltra: "classical Chinese feel",
	}
	for level, want := range cases {
		req := &core.ChatRequest{}
		Apply(req, Config{Enabled: true, Level: level})
		if !strings.Contains(req.System, want) {
			t.Errorf("level %q: expected %q in prompt, got %q", level, want, req.System)
		}
	}
}

func TestEnhancedFullPrompt(t *testing.T) {
	p := promptFor(LevelFull)
	for _, want := range []string{
		"No tool-call narration",
		"no decorative tables",
		"Standard well-known tech acronyms",
		"never invent new abbreviations",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("full prompt missing %q", want)
		}
	}
}

func TestEnhancedUltraPrompt(t *testing.T) {
	p := promptFor(LevelUltra)
	for _, want := range []string{
		"prose words only",
		"never real code symbols",
		"Code symbols, function names, API names",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("ultra prompt missing %q", want)
		}
	}
}

func TestSharedBoundariesEnhanced(t *testing.T) {
	for _, want := range []string{
		"Do not describe or announce this style",
		"Preserve user's dominant language",
		"ALWAYS keep technical terms",
		"CLI commands",
	} {
		if !strings.Contains(sharedBoundaries, want) {
			t.Errorf("sharedBoundaries missing %q", want)
		}
	}
}

func TestSharedAutoClarityEnhanced(t *testing.T) {
	if !strings.Contains(sharedAutoClarity, "compression itself creates technical ambiguity") {
		t.Error("sharedAutoClarity missing technical ambiguity condition")
	}
}
