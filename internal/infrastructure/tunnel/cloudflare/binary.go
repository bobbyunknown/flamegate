// Package cloudflare manages the cloudflared binary and quick tunnel lifecycle.
package cloudflare

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
)

const (
	binaryName       = "cloudflared"
	minBinarySize    = 1 << 20 // 1MB — cloudflared is ~30MB+
	githubReleaseURL = "https://github.com/cloudflare/cloudflared/releases/latest/download"
)

// platformAsset maps GOOS/GOARCH to the cloudflared release asset name.
var platformAsset = map[string]map[string]string{
	"darwin": {
		"amd64": "cloudflared-darwin-amd64.tgz",
		"arm64": "cloudflared-darwin-arm64.tgz",
	},
	"linux": {
		"amd64": "cloudflared-linux-amd64",
		"arm64": "cloudflared-linux-arm64",
	},
	"windows": {
		"amd64": "cloudflared-windows-amd64.exe",
		"386":   "cloudflared-windows-386.exe",
	},
}

// platformFallback is the most-compatible binary per platform.
var platformFallback = map[string]string{
	"darwin":  "cloudflared-darwin-amd64.tgz",
	"linux":   "cloudflared-linux-amd64",
	"windows": "cloudflared-windows-386.exe",
}

// DownloadState tracks binary download progress.
type DownloadState struct {
	Downloading atomic.Bool
	Progress    atomic.Int32
}

var dlState DownloadState

// GetDownloadStatus returns the current download state.
func GetDownloadStatus() (downloading bool, progress int) {
	return dlState.Downloading.Load(), int(dlState.Progress.Load())
}

// BinDir returns the directory where cloudflared is stored.
func BinDir(dataDir string) string {
	return filepath.Join(dataDir, "bin")
}

// BinPath returns the full path to the cloudflared binary.
func BinPath(dataDir string) string {
	name := binaryName
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(BinDir(dataDir), name)
}

// getDownloadURL returns the download URL for the current platform.
func getDownloadURL() (string, bool, error) {
	platform := platformAsset[runtime.GOOS]
	if platform == nil {
		return "", false, fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	asset, ok := platform[runtime.GOARCH]
	if !ok {
		asset = platformFallback[runtime.GOARCH]
		if asset == "" {
			// Use platform's amd64 as last resort.
			for _, v := range platform {
				asset = v
				break
			}
		}
	}
	isArchive := len(asset) > 4 && asset[len(asset)-4:] == ".tgz"
	return githubReleaseURL + "/" + asset, isArchive, nil
}

// isValidBinary checks if the file at path looks like a valid cloudflared
// binary (correct magic bytes, minimum size).
func isValidBinary(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() < minBinarySize {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 4)
	if _, err := io.ReadFull(f, buf); err != nil {
		return false
	}
	magic := fmt.Sprintf("%02x%02x%02x%02x", buf[0], buf[1], buf[2], buf[3])
	switch runtime.GOOS {
	case "windows":
		return magic[:4] == "4d5a" // PE (MZ)
	case "darwin":
		return magic == "cffaedfe" || magic == "cefaedfe" // Mach-O
	default:
		return magic == "7f454c46" // ELF
	}
}

var ensureOnce sync.Once
var ensureErr error

// EnsureCloudflared downloads the cloudflared binary if not present or invalid.
// It is safe to call concurrently; the download happens at most once.
func EnsureCloudflared(dataDir string) (string, error) {
	ensureOnce.Do(func() {
		ensureErr = doEnsure(dataDir)
	})
	return BinPath(dataDir), ensureErr
}

func doEnsure(dataDir string) error {
	binDir := BinDir(dataDir)
	binPath := BinPath(dataDir)

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create bin dir: %w", err)
	}

	// Clean up incomplete downloads.
	tmpPath := binPath + ".tmp"
	os.Remove(tmpPath)

	if _, err := os.Stat(binPath); err == nil {
		if !isValidBinary(binPath) {
		os.Remove(binPath)
		} else {
			if runtime.GOOS != "windows" {
			os.Chmod(binPath, 0o755)
			}
			return nil
		}
	}

	url, isArchive, err := getDownloadURL()
	if err != nil {
		return err
	}

	downloadDest := tmpPath
	if isArchive {
		downloadDest = filepath.Join(binDir, "cloudflared.tgz.tmp")
	}

	if err := downloadFile(url, downloadDest); err != nil {
		return fmt.Errorf("download cloudflared: %w", err)
	}

	if isArchive {
		if err := extractTGZ(downloadDest, binDir); err != nil {
		os.Remove(downloadDest)
			return fmt.Errorf("extract cloudflared: %w", err)
		}
	os.Remove(downloadDest)
	} else {
		if err := os.Rename(downloadDest, binPath); err != nil {
			return fmt.Errorf("rename binary: %w", err)
		}
	}

	if runtime.GOOS != "windows" {
		os.Chmod(binPath, 0o755)
	}
	return nil
}

// downloadFile downloads a URL to dest, following redirects and tracking
// progress via the package-level dlState.
func downloadFile(rawURL, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	dlState.Downloading.Store(true)
	dlState.Progress.Store(0)
	defer func() {
		dlState.Downloading.Store(false)
		dlState.Progress.Store(0)
	}()

	req, err := http.NewRequestWithContext(context.Background(), "GET", rawURL, nil)
	if err != nil {
		os.Remove(dest)
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		_ = os.Remove(dest)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusFound ||
		resp.StatusCode == http.StatusSeeOther || resp.StatusCode == http.StatusTemporaryRedirect ||
		resp.StatusCode == http.StatusPermanentRedirect {
		f.Close()
		os.Remove(dest)
		return downloadFile(resp.Header.Get("Location"), dest)
	}

	if resp.StatusCode != http.StatusOK {
		f.Close()
		os.Remove(dest)
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	totalBytes := resp.ContentLength
	var receivedBytes int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				os.Remove(dest)
				return werr
			}
			receivedBytes += int64(n)
			if totalBytes > 0 {
				dlState.Progress.Store(int32(receivedBytes * 100 / totalBytes))
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			os.Remove(dest)
			return err
		}
	}
	dlState.Progress.Store(100)
	return nil
}

// extractTGZ extracts a .tgz archive to destDir.
func extractTGZ(tgzPath, destDir string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// We only care about the cloudflared binary.
		name := filepath.Base(hdr.Name)
		if name != binaryName && name != binaryName+".exe" {
			continue
		}
		outPath := filepath.Join(destDir, name)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		out.Close()
		return nil
	}
	return fmt.Errorf("cloudflared binary not found in archive")
}
