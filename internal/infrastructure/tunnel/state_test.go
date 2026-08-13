package tunnel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTunnelDirAndFilePaths(t *testing.T) {
	dataDir := "/data"
	if got, want := TunnelDir(dataDir), filepath.Join(dataDir, "tunnel"); got != want {
		t.Fatalf("TunnelDir = %q, want %q", got, want)
	}
	if got, want := StateFile(dataDir), filepath.Join(dataDir, "tunnel", "state.json"); got != want {
		t.Fatalf("StateFile = %q, want %q", got, want)
	}
	if got, want := PIDFile(dataDir), filepath.Join(dataDir, "tunnel", "cloudflared.pid"); got != want {
		t.Fatalf("PIDFile = %q, want %q", got, want)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// No state yet.
	if s := LoadState(dir); s != nil {
		t.Fatalf("LoadState on empty dir = %+v, want nil", s)
	}

	want := &TunnelState{ShortID: "ab23cd", TunnelURL: "https://foo-bar.trycloudflare.com"}
	if err := SaveState(dir, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	got := LoadState(dir)
	if got == nil {
		t.Fatal("LoadState returned nil after save")
	}
	if got.ShortID != want.ShortID || got.TunnelURL != want.TunnelURL {
		t.Fatalf("LoadState = %+v, want %+v", got, want)
	}

	ClearState(dir)
	if s := LoadState(dir); s != nil {
		t.Fatalf("LoadState after clear = %+v, want nil", s)
	}
}

func TestLoadStateCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureTunnelDir(dir); err != nil {
		t.Fatalf("EnsureTunnelDir: %v", err)
	}
	if err := writeFile(StateFile(dir), "{ not json"); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}
	if s := LoadState(dir); s != nil {
		t.Fatalf("LoadState on corrupt file = %+v, want nil", s)
	}
}

func TestPIDRoundTrip(t *testing.T) {
	dir := t.TempDir()

	if pid := LoadPID(dir); pid != 0 {
		t.Fatalf("LoadPID on empty dir = %d, want 0", pid)
	}

	if err := SavePID(dir, 12345); err != nil {
		t.Fatalf("SavePID: %v", err)
	}
	if pid := LoadPID(dir); pid != 12345 {
		t.Fatalf("LoadPID = %d, want 12345", pid)
	}

	ClearPID(dir)
	if pid := LoadPID(dir); pid != 0 {
		t.Fatalf("LoadPID after clear = %d, want 0", pid)
	}
}

func TestLoadPIDPlainText(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureTunnelDir(dir); err != nil {
		t.Fatalf("EnsureTunnelDir: %v", err)
	}
	// A plain-text PID file (not JSON) should still parse.
	if err := writeFile(PIDFile(dir), "  67890\n"); err != nil {
		t.Fatalf("write pid: %v", err)
	}
	if pid := LoadPID(dir); pid != 67890 {
		t.Fatalf("LoadPID plain text = %d, want 67890", pid)
	}
}

func TestGenerateShortID(t *testing.T) {
	allowed := map[rune]bool{}
	for _, c := range shortIDChars {
		allowed[c] = true
	}

	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := GenerateShortID()
		if len(id) != ShortIDLength {
			t.Fatalf("GenerateShortID length = %d, want %d", len(id), ShortIDLength)
		}
		for _, c := range id {
			if !allowed[c] {
				t.Fatalf("GenerateShortID produced disallowed char %q in %q", c, id)
			}
		}
		seen[id] = true
	}
	// Extremely unlikely to collide down to a handful over 200 draws.
	if len(seen) < 100 {
		t.Fatalf("GenerateShortID looks non-random: only %d unique of 200", len(seen))
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
