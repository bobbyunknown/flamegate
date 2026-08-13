package tailscale

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTailscaleDirAndSocket(t *testing.T) {
	dataDir := "/data"
	if got, want := TailscaleDir(dataDir), filepath.Join(dataDir, "tailscale"); got != want {
		t.Fatalf("TailscaleDir = %q, want %q", got, want)
	}
	if got, want := TailscaleSocket(dataDir), filepath.Join(dataDir, "tailscale", "tailscaled.sock"); got != want {
		t.Fatalf("TailscaleSocket = %q, want %q", got, want)
	}
}

func TestFindBinaryCustom(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("custom-binary path uses unix naming in this test")
	}
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	custom := filepath.Join(binDir, "tailscale")
	if err := os.WriteFile(custom, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write custom binary: %v", err)
	}
	if got := FindBinary(dir); got != custom {
		t.Fatalf("FindBinary = %q, want custom %q", got, custom)
	}
	if !IsInstalled(dir) {
		t.Fatal("IsInstalled = false with custom binary present")
	}
}

func TestFindBinaryNoneInEmptyDir(t *testing.T) {
	// An empty data dir with no custom binary falls back to PATH/well-known.
	// We can't assert absence reliably (host may have tailscale installed),
	// but the custom-binary path must not resolve inside an empty temp dir.
	dir := t.TempDir()
	got := FindBinary(dir)
	custom := filepath.Join(dir, "bin", "tailscale")
	if got == custom {
		t.Fatalf("FindBinary resolved to non-existent custom binary %q", got)
	}
}

func TestValidateSudoPasswordRejectsInvalid(t *testing.T) {
	if err := ValidateSudoPassword(""); err == nil {
		t.Fatal("empty password accepted, want error")
	}
	if err := ValidateSudoPassword("has\nnewline"); err == nil {
		t.Fatal("password with newline accepted, want error")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "x")
	if fileExists(f) {
		t.Fatal("fileExists(missing) = true")
	}
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !fileExists(f) {
		t.Fatal("fileExists(present) = false")
	}
}
