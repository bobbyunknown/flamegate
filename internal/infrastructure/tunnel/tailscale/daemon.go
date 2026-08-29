package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// DaemonStatus holds the state of the tailscaled daemon.
type DaemonStatus struct {
	BackendState string `json:"BackendState"`
	Self         *struct {
		Online  bool   `json:"Online"`
		DNSName string `json:"DNSName"`
	} `json:"Self,omitempty"`
	AuthURL string `json:"AuthURL,omitempty"`
}

// tsArgs builds tailscale CLI args with the custom socket flag.
func tsArgs(dataDir string, args ...string) []string {
	if runtime.GOOS == "windows" {
		return args
	}
	return append([]string{"--socket", TailscaleSocket(dataDir)}, args...)
}

// StartDaemon starts the tailscaled daemon. With sudoPassword, it runs in TUN
// mode (root). Without, it falls back to userspace-networking.
func StartDaemon(dataDir string, sudoPassword string, log *logrus.Logger) error {
	if runtime.GOOS == "windows" {
		return startDaemonWindows(log)
	}

	tsDir := TailscaleDir(dataDir)
	socket := TailscaleSocket(dataDir)
	EnsureUserOwnedDir(tsDir)

	wantTun := sudoPassword != ""
	currentMode := isDaemonTunMode(socket)

	// Daemon already running in correct mode → reuse.
	if currentMode != nil && *currentMode == wantTun {
		if isDaemonResponsive(dataDir) {
			return nil
		}
	}

	// Kill existing daemons on our socket.
	killDaemon(socket, sudoPassword)
	time.Sleep(1500 * time.Millisecond)

	// Reclaim folder ownership.
	EnsureUserOwnedDir(tsDir)

	tailscaledBin := "tailscaled"
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/usr/local/bin/tailscaled"); err == nil {
			tailscaledBin = "/usr/local/bin/tailscaled"
		}
	}

	daemonArgs := []string{
		fmt.Sprintf("--socket=%s", socket),
		fmt.Sprintf("--statedir=%s", tsDir),
	}
	if !wantTun {
		daemonArgs = append(daemonArgs, "--tun=userspace-networking")
	}

	if wantTun {
		// Validate sudo password before spawning detached daemon.
		// Without this, a wrong password silently fails and the daemon never starts.
		if err := ValidateSudoPassword(sudoPassword); err != nil {
			return fmt.Errorf("cannot start TUN daemon: %w", err)
		}

		// TUN mode: spawn via sudo with password via stdin.
		sudoArgs := append([]string{"-S", tailscaledBin}, daemonArgs...)
		cmd := exec.Command("sudo", sudoArgs...)
		cmd.Dir = os.TempDir()
		cmd.Env = append(os.Environ(), extendedPath())
		stdin, _ := cmd.StdinPipe()
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start tailscaled (TUN): %w", err)
		}
	fmt.Fprintf(stdin, "%s\n", sudoPassword)
		stdin.Close()
		cmd.Run()
	} else {
		cmd := exec.Command(tailscaledBin, daemonArgs...)
		cmd.Dir = os.TempDir()
		cmd.Env = append(os.Environ(), extendedPath())
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start tailscaled: %w", err)
		}
		go cmd.Wait()
	}

	// Wait for socket to be ready.
	time.Sleep(3 * time.Second)
	log.Info("[Tailscale] daemon started", "tun", wantTun)
	return nil
}

func startDaemonWindows(log *logrus.Logger) error {
	// Windows: tailscale runs as a Windows Service.
	exec.Command("net", "start", "Tailscale").Run()

	bin := FindBinary("")
	if bin == "" {
		return nil
	}

	// Poll until daemon finishes init.
	for i := 0; i < 20; i++ {
		out, err := exec.Command(bin, "status", "--json").Output()
		if err == nil {
			var status DaemonStatus
			if json.Unmarshal(out, &status) == nil && status.BackendState != "" && status.BackendState != "NoState" {
				log.Info("[Tailscale] windows daemon ready", "state", status.BackendState)
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

// StopDaemon kills the tailscaled daemon.
func StopDaemon(dataDir string, sudoPassword string) {
	socket := TailscaleSocket(dataDir)
	killDaemon(socket, sudoPassword)
	os.Remove(socket)
}

func killDaemon(socket string, sudoPassword string) {
	// Try non-sudo first.
	exec.Command("pkill", "-9", "-f", fmt.Sprintf("tailscaled.*%s", socket)).Run()

	// Check if still alive.
	if err := exec.Command("pgrep", "-f", fmt.Sprintf("tailscaled.*%s", socket)).Run(); err != nil {
		return // Dead.
	}

	// Kill with sudo.
	if sudoPassword != "" {
		cmd := exec.Command("sudo", "-S", "pkill", "-9", "-f", fmt.Sprintf("tailscaled.*%s", socket))
		stdin, _ := cmd.StdinPipe()
		if err := cmd.Start(); err == nil {
		fmt.Fprintf(stdin, "%s\n", sudoPassword)
			stdin.Close()
		cmd.Wait()
		}
	} else {
		exec.Command("sudo", "-n", "pkill", "-9", "-f", fmt.Sprintf("tailscaled.*%s", socket)).Run()
	}
}

// isDaemonTunMode checks if the running daemon uses TUN mode.
// Returns nil if daemon is not running.
func isDaemonTunMode(socket string) *bool {
	out, err := exec.Command("pgrep", "-af", fmt.Sprintf("tailscaled.*%s", socket)).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return nil
	}
	isUserspace := strings.Contains(string(out), "--tun=userspace-networking")
	tun := !isUserspace
	return &tun
}

// isDaemonResponsive checks if the daemon responds to status queries.
func isDaemonResponsive(dataDir string) bool {
	bin := FindBinary(dataDir)
	if bin == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, tsArgs(dataDir, "status", "--json")...)
	cmd.Env = append(os.Environ(), extendedPath())
	return cmd.Run() == nil
}

// GetStatus queries `tailscale status --json` and returns the parsed status.
func GetStatus(dataDir string) (*DaemonStatus, error) {
	bin := FindBinary(dataDir)
	if bin == "" {
		return nil, fmt.Errorf("tailscale not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, tsArgs(dataDir, "status", "--json")...)
	cmd.Env = append(os.Environ(), extendedPath())
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var status DaemonStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// IsLoggedIn checks if the device is logged into a tailnet.
func IsLoggedIn(dataDir string) bool {
	status, err := GetStatus(dataDir)
	if err != nil {
		return false
	}
	return status.BackendState == "Running" && status.Self != nil && status.Self.Online
}

// GetFunnelURL returns the funnel URL from tailscale status.
func GetFunnelURL(dataDir string) string {
	status, err := GetStatus(dataDir)
	if err != nil || status.Self == nil {
		return ""
	}
	dnsName := strings.TrimSuffix(status.Self.DNSName, ".")
	if dnsName == "" {
		return ""
	}
	return "https://" + dnsName
}

func extendedPath() string {
	return fmt.Sprintf("PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:%s", os.Getenv("PATH"))
}
