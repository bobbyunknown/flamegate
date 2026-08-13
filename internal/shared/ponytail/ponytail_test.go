package ponytail

import (
	"strings"
	"testing"

	"github.com/bobbyunknown/flamegate/internal/domain"
)

func TestValidLevel(t *testing.T) {
	for _, l := range []Level{LevelLite, LevelFull, LevelUltra} {
		if !ValidLevel(l) {
			t.Errorf("ValidLevel(%q) = false, want true", l)
		}
	}
	for _, l := range []Level{"", "LITE", "medium", "Full"} {
		if ValidLevel(l) {
			t.Errorf("ValidLevel(%q) = true, want false", l)
		}
	}
}

func TestApplyDisabledIsNoop(t *testing.T) {
	req := &core.ChatRequest{System: "original"}
	Apply(req, Config{Enabled: false, Level: LevelFull})
	if req.System != "original" {
		t.Fatalf("disabled Apply modified system: %q", req.System)
	}
}

func TestApplyNilRequest(t *testing.T) {
	// Must not panic.
	Apply(nil, Config{Enabled: true, Level: LevelFull})
}

func TestApplyEmptySystem(t *testing.T) {
	req := &core.ChatRequest{System: ""}
	Apply(req, Config{Enabled: true, Level: LevelFull})
	if !strings.Contains(req.System, sentinel) {
		t.Fatalf("sentinel not injected: %q", req.System)
	}
	if !strings.Contains(req.System, "lazy senior developer") {
		t.Fatalf("prompt body missing: %q", req.System)
	}
}

func TestApplyAppendsAfterExisting(t *testing.T) {
	req := &core.ChatRequest{System: "KEEP THIS"}
	Apply(req, Config{Enabled: true, Level: LevelFull})
	if !strings.HasPrefix(req.System, "KEEP THIS") {
		t.Fatalf("existing system text not preserved at front: %q", req.System)
	}
	if !strings.Contains(req.System, sentinel) {
		t.Fatal("sentinel not appended")
	}
}

func TestApplyIdempotent(t *testing.T) {
	req := &core.ChatRequest{System: "base"}
	Apply(req, Config{Enabled: true, Level: LevelUltra})
	afterFirst := req.System
	Apply(req, Config{Enabled: true, Level: LevelUltra})
	if req.System != afterFirst {
		t.Fatalf("second Apply changed system (not idempotent):\nfirst:  %q\nsecond: %q", afterFirst, req.System)
	}
	if strings.Count(req.System, sentinel) != 1 {
		t.Fatalf("sentinel appears %d times, want 1", strings.Count(req.System, sentinel))
	}
}

func TestApplyInvalidLevelFallsBackToFull(t *testing.T) {
	req := &core.ChatRequest{}
	Apply(req, Config{Enabled: true, Level: Level("bogus")})
	if !strings.Contains(req.System, levelLines[LevelFull]) {
		t.Fatalf("invalid level did not fall back to Full: %q", req.System)
	}
}

func TestApplyLevelSpecificLine(t *testing.T) {
	cases := []Level{LevelLite, LevelFull, LevelUltra}
	for _, l := range cases {
		req := &core.ChatRequest{}
		Apply(req, Config{Enabled: true, Level: l})
		if !strings.Contains(req.System, levelLines[l]) {
			t.Errorf("level %q missing its specific line", l)
		}
	}
}
