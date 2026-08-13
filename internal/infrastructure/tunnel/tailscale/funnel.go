package tailscale

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var loginURLRegex = regexp.MustCompile(`https://login\.tailscale\.com/a/[a-zA-Z0-9]+`)
var enableURLRegex = regexp.MustCompile(`https://login\.tailscale\.com/[^\s]+`)

// LoginResult holds the result of a tailscale login attempt.
type LoginResult struct {
	AlreadyLoggedIn bool   `json:"alreadyLoggedIn,omitempty"`
	AuthURL         string `json:"authUrl,omitempty"`
}

// FunnelResult holds the result of starting a funnel.
type FunnelResult struct {
	TunnelURL        string `json:"tunnelUrl,omitempty"`
	FunnelNotEnabled bool   `json:"funnelNotEnabled,omitempty"`
	EnableURL        string `json:"enableUrl,omitempty"`
}

// StartLogin runs `tailscale up` and captures the auth URL for browser login.
func StartLogin(dataDir string, hostname string) (*LoginResult, error) {
	bin := FindBinary(dataDir)
	if bin == "" {
		return nil, fmt.Errorf("tailscale not installed")
	}

	// Ensure daemon is running (best-effort, no sudo).
	go StartDaemon(dataDir, "", logrus.StandardLogger()) //nolint:errcheck // background daemon

	// Check if already logged in.
	if IsLoggedIn(dataDir) {
		return &LoginResult{AlreadyLoggedIn: true}, nil
	}

	args := tsArgs(dataDir, "up", "--accept-routes")
	if hostname != "" {
		args = append(args, "--hostname="+hostname)
	}

	cmd := exec.Command(bin, args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("tailscale up: %w", err)
	}

	var output strings.Builder
	done := make(chan struct{})
	var result *LoginResult

	// Parse output for auth URL.
	go func() {
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		for scanner.Scan() {
			line := scanner.Text()
			output.WriteString(line + "\n")
			if match := loginURLRegex.FindString(line); match != "" {
				result = &LoginResult{AuthURL: match}
				close(done)
				return
			}
		}
		close(done)
	}()

	// Also poll status for AuthURL (Windows compatibility).
	statusPoll := time.NewTicker(500 * time.Millisecond)
	defer statusPoll.Stop()

	timeout := time.After(15 * time.Second)

	for {
		select {
		case <-done:
			if result != nil {
				cmd.Process.Kill() //nolint:errcheck // best-effort kill
				return result, nil
			}
			// Process exited without auth URL.
			if IsLoggedIn(dataDir) {
				return &LoginResult{AlreadyLoggedIn: true}, nil
			}
			// Check status one more time.
			if status, err := GetStatus(dataDir); err == nil && status.AuthURL != "" {
				return &LoginResult{AuthURL: status.AuthURL}, nil
			}
			return nil, fmt.Errorf("tailscale up completed without auth URL")
		case <-statusPoll.C:
			if status, err := GetStatus(dataDir); err == nil && status.AuthURL != "" {
				result = &LoginResult{AuthURL: status.AuthURL}
				cmd.Process.Kill() //nolint:errcheck // best-effort kill
				return result, nil
			}
		case <-timeout:
			cmd.Process.Kill() //nolint:errcheck // best-effort kill
			// Final check.
			if IsLoggedIn(dataDir) {
				return &LoginResult{AlreadyLoggedIn: true}, nil
			}
			if status, err := GetStatus(dataDir); err == nil && status.AuthURL != "" {
				return &LoginResult{AuthURL: status.AuthURL}, nil
			}
			return nil, fmt.Errorf("tailscale up timed out without auth URL")
		}
	}
}

// StartFunnel starts the Tailscale funnel for the given port.
func StartFunnel(dataDir string, port int) (*FunnelResult, error) {
	bin := FindBinary(dataDir)
	if bin == "" {
		return nil, fmt.Errorf("tailscale not installed")
	}

	// Reset existing funnel.
	exec.Command(bin, tsArgs(dataDir, "funnel", "--bg", "reset")...).Run() //nolint:errcheck // best-effort exec

	args := tsArgs(dataDir, "funnel", "--bg", fmt.Sprintf("%d", port))
	cmd := exec.Command(bin, args...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start funnel: %w", err)
	}

	var output strings.Builder
	done := make(chan struct{})
	var result *FunnelResult

	go func() {
		scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
		funnelNotEnabled := false
		for scanner.Scan() {
			line := scanner.Text()
			output.WriteString(line + "\n")

			if strings.Contains(line, "Funnel is not enabled") {
				funnelNotEnabled = true
			}
			if funnelNotEnabled {
				if match := enableURLRegex.FindString(line); match != "" {
					result = &FunnelResult{FunnelNotEnabled: true, EnableURL: match}
					close(done)
					return
				}
			}
		}
		close(done)
	}()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-done:
			if result != nil {
				cmd.Process.Kill() //nolint:errcheck // best-effort kill
				return result, nil
			}
			// Process exited — try to get URL from status.
			if url := GetFunnelURL(dataDir); url != "" {
				return &FunnelResult{TunnelURL: url}, nil
			}
			return nil, fmt.Errorf("funnel exited without URL: %s", strings.TrimSpace(output.String()))
		case <-timeout:
			cmd.Process.Kill() //nolint:errcheck // best-effort kill
			// --bg exits after setup, read actual hostname from status.
			if url := GetFunnelURL(dataDir); url != "" {
				return &FunnelResult{TunnelURL: url}, nil
			}
			return nil, fmt.Errorf("funnel timed out: %s", strings.TrimSpace(output.String()))
		}
	}
}

// StopFunnel stops the Tailscale funnel.
func StopFunnel(dataDir string) {
	bin := FindBinary(dataDir)
	if bin == "" {
		return
	}
	exec.Command(bin, tsArgs(dataDir, "funnel", "--bg", "reset")...).Run() //nolint:errcheck // best-effort exec
}

// ProvisionCert provisions a TLS certificate for the funnel domain.
// This is best-effort and non-fatal.
func ProvisionCert(dataDir string, hostname string, log *logrus.Logger) {
	bin := FindBinary(dataDir)
	if bin == "" || hostname == "" {
		return
	}
	certsDir := filepath.Join(TailscaleDir(dataDir), "certs")
	os.MkdirAll(certsDir, 0o700) //nolint:errcheck // best-effort mkdir
	certFile := filepath.Join(certsDir, hostname+".crt")
	keyFile := filepath.Join(certsDir, hostname+".key")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, tsArgs(dataDir, "cert",
		"--cert-file", certFile, "--key-file", keyFile, hostname)...)
	cmd.Env = append(os.Environ(), extendedPath())
	if err := cmd.Run(); err != nil {
		log.Warn("[Tailscale] cert provision failed (non-fatal)", "error", err)
	} else {
		log.Info("[Tailscale] cert provisioned", "hostname", hostname)
	}
}
