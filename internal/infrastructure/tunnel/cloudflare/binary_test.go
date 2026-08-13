package cloudflare

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBinDirAndBinPath(t *testing.T) {
	dataDir := "/data"
	if got, want := BinDir(dataDir), filepath.Join(dataDir, "bin"); got != want {
		t.Fatalf("BinDir = %q, want %q", got, want)
	}
	got := BinPath(dataDir)
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(got, "cloudflared.exe") {
			t.Fatalf("BinPath = %q, want .exe suffix on windows", got)
		}
	} else if !strings.HasSuffix(got, "cloudflared") {
		t.Fatalf("BinPath = %q, want cloudflared suffix", got)
	}
}

func TestGetDownloadURL(t *testing.T) {
	url, isArchive, err := getDownloadURL()
	if err != nil {
		// Only unsupported platforms error; the test host should be supported.
		t.Skipf("platform %s/%s unsupported: %v", runtime.GOOS, runtime.GOARCH, err)
	}
	if !strings.HasPrefix(url, githubReleaseURL+"/") {
		t.Fatalf("download URL %q missing release prefix", url)
	}
	// Archive flag must agree with the .tgz suffix (only darwin uses archives).
	if isArchive != strings.HasSuffix(url, ".tgz") {
		t.Fatalf("isArchive=%v but url=%q", isArchive, url)
	}
	if runtime.GOOS == "darwin" && !isArchive {
		t.Fatalf("darwin asset should be an archive: %q", url)
	}
}

func TestIsValidBinary(t *testing.T) {
	dir := t.TempDir()

	// Missing file.
	if isValidBinary(filepath.Join(dir, "nope")) {
		t.Fatal("isValidBinary(missing) = true, want false")
	}

	// Too small.
	small := filepath.Join(dir, "small")
	if err := os.WriteFile(small, []byte("tiny"), 0o644); err != nil {
		t.Fatalf("write small: %v", err)
	}
	if isValidBinary(small) {
		t.Fatal("isValidBinary(small) = true, want false")
	}

	// Large enough with correct magic bytes for the current platform.
	var magic []byte
	switch runtime.GOOS {
	case "windows":
		magic = []byte{0x4d, 0x5a, 0x90, 0x00} // MZ
	case "darwin":
		magic = []byte{0xcf, 0xfa, 0xed, 0xfe} // Mach-O 64
	default:
		magic = []byte{0x7f, 0x45, 0x4c, 0x46} // ELF
	}
	good := filepath.Join(dir, "good")
	buf := make([]byte, minBinarySize+16)
	copy(buf, magic)
	if err := os.WriteFile(good, buf, 0o644); err != nil {
		t.Fatalf("write good: %v", err)
	}
	if !isValidBinary(good) {
		t.Fatal("isValidBinary(valid) = false, want true")
	}

	// Large enough but wrong magic bytes.
	bad := filepath.Join(dir, "bad")
	buf2 := make([]byte, minBinarySize+16)
	copy(buf2, []byte{0x00, 0x11, 0x22, 0x33})
	if err := os.WriteFile(bad, buf2, 0o644); err != nil {
		t.Fatalf("write bad: %v", err)
	}
	if isValidBinary(bad) {
		t.Fatal("isValidBinary(wrong magic) = true, want false")
	}
}

func TestGetDownloadStatusDefault(t *testing.T) {
	downloading, progress := GetDownloadStatus()
	if downloading {
		t.Fatal("GetDownloadStatus reports downloading at rest")
	}
	if progress != 0 {
		t.Fatalf("GetDownloadStatus progress = %d, want 0", progress)
	}
}
