// Package tailscale manages the Tailscale binary, daemon, and funnel lifecycle.
package tailscale

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// wellKnownPaths are common Tailscale install locations on Unix.
var wellKnownPaths = []string{
	"/usr/local/bin/tailscale",
	"/opt/homebrew/bin/tailscale",
	"/usr/bin/tailscale",
}

const windowsTailscaleBin = `C:\Program Files\Tailscale\tailscale.exe`

// FindBinary locates the tailscale binary. It checks:
// 1. Custom binary in data_dir/bin/tailscale
// 2. Well-known install paths
// 3. System PATH via exec.LookPath
func FindBinary(dataDir string) string {
	// Custom binary.
	custom := filepath.Join(dataDir, "bin", "tailscale")
	if runtime.GOOS == "windows" {
		custom += ".exe"
	}
	if fileExists(custom) {
		return custom
	}

	// Windows default.
	if runtime.GOOS == "windows" && fileExists(windowsTailscaleBin) {
		return windowsTailscaleBin
	}

	// Well-known Unix paths.
	if runtime.GOOS != "windows" {
		for _, p := range wellKnownPaths {
			if fileExists(p) {
				return p
			}
		}
	}

	// System PATH.
	if path, err := exec.LookPath("tailscale"); err == nil {
		return path
	}

	return ""
}

// IsInstalled returns true if the tailscale binary is found.
func IsInstalled(dataDir string) bool {
	return FindBinary(dataDir) != ""
}

// TailscaleSocket returns the path to the custom tailscaled socket.
func TailscaleSocket(dataDir string) string {
	return filepath.Join(TailscaleDir(dataDir), "tailscaled.sock")
}

// TailscaleDir returns the directory for Tailscale state.
func TailscaleDir(dataDir string) string {
	return filepath.Join(dataDir, "tailscale")
}

// InstallTailscale installs Tailscale on the current platform.
// On macOS with brew, no sudo password is needed. On Linux/macOS without brew,
// sudoPassword is required. On Windows, UAC elevation is used.
// onProgress receives progress messages.
func InstallTailscale(dataDir string, sudoPassword string, onProgress func(string)) error {
	log := onProgress
	if log == nil {
		log = func(string) {}
	}

	if runtime.GOOS == "windows" {
		return installWindows(log)
	}
	if runtime.GOOS == "darwin" && hasBrew() {
		return installMacBrew(log)
	}
	if runtime.GOOS == "darwin" {
		return installMacPkg(sudoPassword, log)
	}
	return installLinux(sudoPassword, log)
}

func hasBrew() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

func installMacBrew(log func(string)) error {
	log("Installing via Homebrew...")
	cmd := exec.Command("brew", "install", "tailscale")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("brew install: %w", err)
	}
	go streamOutput(stdout, log)
	go streamOutput(stderr, log)
	return cmd.Wait()
}

func installMacPkg(sudoPassword string, log func(string)) error {
	if sudoPassword == "" {
		return fmt.Errorf("sudo password is required for macOS pkg install")
	}
	if strings.Contains(sudoPassword, "\n") {
		return fmt.Errorf("invalid sudo password")
	}

	pkgURL := "https://pkgs.tailscale.com/stable/tailscale-latest.pkg"
	pkgPath := filepath.Join(os.TempDir(), "tailscale.pkg")

	log("Downloading Tailscale package...")
	if err := downloadToFile(pkgURL, pkgPath, log); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer os.Remove(pkgPath) //nolint:errcheck // best-effort cleanup

	log("Installing package...")
	cmd := exec.Command("sudo", "-S", "installer", "-pkg", pkgPath, "-target", "/")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start installer: %w", err)
	}
	go streamOutput(stdout, log)
	var stderrBuf strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrBuf.WriteString(scanner.Text() + "\n")
		}
	}()

	fmt.Fprintf(stdin, "%s\n", sudoPassword) //nolint:errcheck // best-effort
	stdin.Close() //nolint:errcheck // best-effort close //nolint:errcheck // best-effort

	if err := cmd.Wait(); err != nil {
		errOut := stderrBuf.String()
		if strings.Contains(errOut, "incorrect password") || strings.Contains(errOut, "Sorry") {
			return fmt.Errorf("wrong sudo password")
		}
		return fmt.Errorf("installer failed: %s", errOut)
	}
	return nil
}

func installLinux(sudoPassword string, log func(string)) error {
	if sudoPassword == "" {
		return fmt.Errorf("sudo password is required for Linux install")
	}
	if strings.Contains(sudoPassword, "\n") {
		return fmt.Errorf("invalid sudo password")
	}

	log("Downloading install script...")
	req, err := http.NewRequestWithContext(context.Background(), "GET", "https://tailscale.com/install.sh", nil)
	if err != nil {
		return fmt.Errorf("create install script request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download install script: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close //nolint:errcheck // best-effort close
	scriptContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read install script: %w", err)
	}

	// Security: write to temp file, never pipe through stdin.
	tmpScript := filepath.Join(os.TempDir(), fmt.Sprintf("tailscale-install-%d.sh", time.Now().UnixNano()))
	if err := os.WriteFile(tmpScript, scriptContent, 0o700); err != nil {
		return fmt.Errorf("write install script: %w", err)
	}
	defer os.Remove(tmpScript) //nolint:errcheck // best-effort cleanup

	log("Running install script...")
	cmd := exec.Command("sudo", "-S", "sh", tmpScript)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start install: %w", err)
	}
	go streamOutput(stdout, log)
	var stderrBuf strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrBuf.WriteString(scanner.Text() + "\n")
		}
	}()

	fmt.Fprintf(stdin, "%s\n", sudoPassword) //nolint:errcheck // best-effort
	stdin.Close() //nolint:errcheck // best-effort close //nolint:errcheck // best-effort

	if err := cmd.Wait(); err != nil {
		errOut := stderrBuf.String()
		if strings.Contains(errOut, "incorrect password") || strings.Contains(errOut, "Sorry") {
			return fmt.Errorf("wrong sudo password")
		}
		return fmt.Errorf("install failed: %s", errOut)
	}
	return nil
}

func installWindows(log func(string)) error {
	msiURL := "https://pkgs.tailscale.com/stable/tailscale-setup-latest-amd64.msi"
	msiPath := filepath.Join(os.TempDir(), "tailscale-setup.msi")

	log("Downloading Tailscale installer...")
	if err := downloadToFile(msiURL, msiPath, log); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer os.Remove(msiPath) //nolint:errcheck // best-effort cleanup

	log("Installing Tailscale (UAC prompt may appear)...")
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		fmt.Sprintf(`Start-Process msiexec -ArgumentList '/i','%s','TS_NOLAUNCH=true','/quiet','/norestart' -Verb RunAs -Wait`, msiPath))
	stderr, _ := cmd.StderrPipe()
	go streamOutput(stderr, log)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("msiexec failed: %w", err)
	}

	// Verify installation.
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			return fmt.Errorf("installation finished but tailscale.exe not found")
		default:
		}
		if fileExists(windowsTailscaleBin) {
			log("Installation complete.")
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}

func downloadToFile(url, path string, log func(string)) error {
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close //nolint:errcheck // best-effort close
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // best-effort close
	total := resp.ContentLength
	var received int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			received += int64(n)
			if total > 0 && log != nil {
				pct := received * 100 / total
				log(fmt.Sprintf("Downloading... %d%%", pct))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

func streamOutput(r io.Reader, log func(string)) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			log(line)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// HasBrew returns true if Homebrew is available on the system.
func HasBrew() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

// ValidateSudoPassword checks whether the given password is a valid sudo
// credential by running `sudo -S true`. Returns nil on success, error on
// wrong password or other sudo failure.
func ValidateSudoPassword(sudoPassword string) error {
	if sudoPassword == "" {
		return fmt.Errorf("sudo password is required")
	}
	if strings.Contains(sudoPassword, "\n") {
		return fmt.Errorf("invalid sudo password")
	}

	cmd := exec.Command("sudo", "-S", "true")
	stdin, _ := cmd.StdinPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("sudo not available: %w", err)
	}

	fmt.Fprintf(stdin, "%s\n", sudoPassword) //nolint:errcheck // best-effort
	stdin.Close() //nolint:errcheck // best-effort close //nolint:errcheck // best-effort

	var stderrBuf strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrBuf.WriteString(scanner.Text() + "\n")
		}
	}()

	if err := cmd.Wait(); err != nil {
		errOut := stderrBuf.String()
		if strings.Contains(errOut, "incorrect password") || strings.Contains(errOut, "Sorry") {
			return fmt.Errorf("wrong sudo password")
		}
		return fmt.Errorf("sudo validation failed: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// EnsureUserOwnedDir reclaims ownership of dir if a previous root daemon left
// files behind. Best-effort, non-fatal.
func EnsureUserOwnedDir(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		os.MkdirAll(dir, 0o700) //nolint:errcheck // best-effort
		return
	}
	// Try chown (works if already owned by user).
	uid := os.Getuid()
	gid := os.Getgid()
	cmd := exec.Command("chown", "-R", fmt.Sprintf("%d:%d", uid, gid), dir)
	if err := cmd.Run(); err != nil {
		// Try passwordless sudo.
		cmd = exec.Command("sudo", "-n", "chown", "-R", fmt.Sprintf("%d:%d", uid, gid), dir)
		cmd.Run() //nolint:errcheck // best-effort
	}
}
