package cloudflare

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel"
)

// urlRegex matches the quick tunnel URL. The pattern requires the hostname to
// contain at least one hyphen (all generated tunnel hostnames do) and to be
// preceded by whitespace or start-of-line to avoid matching informational text
// like "trycloudflare.com".
var urlRegex = regexp.MustCompile(`(?:^|\s)(https://([a-z0-9]+-[a-z0-9-]+)\.trycloudflare\.com)`)

// QuickTunnelResult holds the result of spawning a quick tunnel.
type QuickTunnelResult struct {
	Cmd       *exec.Cmd
	TunnelURL string
}

// SpawnQuickTunnel starts cloudflared as a quick tunnel (no account needed)
// pointing at the given local port. It returns the generated trycloudflare.com URL.
func SpawnQuickTunnel(dataDir string, localPort int, log *logrus.Logger) (*QuickTunnelResult, error) {
	binPath, err := EnsureCloudflared(dataDir)
	if err != nil {
		return nil, err
	}

	protocol := os.Getenv("TUNNEL_TRANSPORT_PROTOCOL")
	if protocol == "" {
		protocol = os.Getenv("CLOUDFLARED_PROTOCOL")
	}
	if protocol == "" {
		protocol = "http2"
	}

	args := []string{
		"tunnel",
		"--url", fmt.Sprintf("http://127.0.0.1:%d", localPort),
		"--no-autoupdate",
		"--retries", "99",
	}

	cmd := exec.Command(binPath, args...)
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "TUNNEL_TRANSPORT_PROTOCOL="+protocol)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start cloudflared: %w", err)
	}

	// Save PID.
	_ = tunnel.SavePID(dataDir, cmd.Process.Pid)

	// Parse URL from logs.
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	var logTail strings.Builder
	var mu sync.Mutex

	parseLog := func(data string) {
		mu.Lock()
		logTail.WriteString(data)
		// Keep only last 4000 chars.
		s := logTail.String()
		if len(s) > 4000 {
			logTail.Reset()
			logTail.WriteString(s[len(s)-4000:])
		}
		mu.Unlock()

		matches := urlRegex.FindAllStringSubmatch(data, -1)
		for _, m := range matches {
			// m[1] is the full URL, m[2] is the hostname part.
			tunnelURL := m[1]
			host := m[2]
			if host == "api" {
				continue
			}
			select {
			case resultCh <- tunnelURL:
			default:
			}
		}
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			parseLog(scanner.Text() + "\n")
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			parseLog(scanner.Text() + "\n")
		}
	}()

	go func() {
		err := cmd.Wait()
		tunnel.ClearPID(dataDir)
		// Delay error reporting: if we already got a URL, the process exiting
		// is fine (quick tunnels can die and respawn). Give URL detection a
		// moment to complete before signaling the error.
		time.Sleep(2 * time.Second)
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	// Wait for URL or timeout. Prefer URL over error — the process may exit
	// after emitting the URL (e.g. retry loop).
	timeout := time.After(90 * time.Second)
	for {
		select {
		case tunnelURL := <-resultCh:
			log.Info("[Tunnel] cloudflared URL detected", "url", tunnelURL)
			return &QuickTunnelResult{Cmd: cmd, TunnelURL: tunnelURL}, nil
		case err := <-errCh:
			// Double-check: did we get a URL already?
			select {
			case tunnelURL := <-resultCh:
				log.Info("[Tunnel] cloudflared URL detected (after exit)", "url", tunnelURL)
				return &QuickTunnelResult{Cmd: cmd, TunnelURL: tunnelURL}, nil
			default:
			}
			mu.Lock()
			tail := logTail.String()
			mu.Unlock()
			if len(tail) > 600 {
				tail = tail[len(tail)-600:]
			}
			return nil, fmt.Errorf("cloudflared exited: %v (last log: %s)", err, strings.TrimSpace(tail))
		case <-timeout:
			cmd.Process.Kill()
			mu.Lock()
			tail := logTail.String()
			mu.Unlock()
			if len(tail) > 800 {
				tail = tail[len(tail)-800:]
			}
			return nil, fmt.Errorf("quick tunnel timed out. Last log: %s", strings.TrimSpace(tail))
		}
	}
}

// KillCloudflared kills cloudflared processes. It tries the tracked PID first,
// then falls back to killing by port.
func KillCloudflared(dataDir string, localPort int) {
	// Kill by PID.
	pid := tunnel.LoadPID(dataDir)
	if pid > 0 {
		if p, err := os.FindProcess(pid); err == nil {
			p.Kill()
		}
		tunnel.ClearPID(dataDir)
	}

	// Kill by port (handles orphaned processes).
	if localPort > 0 {
		killByPort(localPort)
	}
}

func killByPort(port int) {
	if runtime.GOOS == "windows" {
		// PowerShell: find and kill cloudflared processes targeting the port.
		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command",
			fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "Name='cloudflared.exe'" | Where-Object { $_.CommandLine -match ':%d(\D|$)' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force }`, port))
		cmd.Run()
	} else {
		cmd := exec.Command("pkill", "-f", fmt.Sprintf("cloudflared.*:%d([^0-9]|$)", port))
		_ = cmd.Run()
	}
}

// IsCloudflaredRunning checks if the tracked cloudflared process is still alive.
func IsCloudflaredRunning(dataDir string) bool {
	pid := tunnel.LoadPID(dataDir)
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks existence without killing the process.
	return p.Signal(syscall.Signal(0)) == nil
}
