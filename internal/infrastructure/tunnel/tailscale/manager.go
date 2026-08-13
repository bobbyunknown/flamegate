package tailscale

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel"
)

// Manager orchestrates the Tailscale funnel lifecycle.
type Manager struct {
	dataDir   string
	localPort int
	log       *logrus.Logger

	mu              sync.Mutex
	cancelToken     chan struct{}
	spawnInProgress bool
}

// NewManager creates a new Tailscale tunnel manager.
func NewManager(dataDir string, localPort int, log *logrus.Logger) *Manager {
	return &Manager{
		dataDir:   dataDir,
		localPort: localPort,
		log:       log,
	}
}

// TailscaleStatus holds the current Tailscale state.
type TailscaleStatus struct {
	Enabled         bool   `json:"enabled"`
	SettingsEnabled bool   `json:"settingsEnabled"`
	TunnelURL       string `json:"tunnelUrl"`
	Running         bool   `json:"running"`
	LoggedIn        bool   `json:"loggedIn"`
	Installed       bool   `json:"installed"`
	Platform        string `json:"platform"`
}

// EnableResult holds the result of enabling Tailscale funnel.
type EnableResult struct {
	Success          bool   `json:"success"`
	TunnelURL        string `json:"tunnelUrl,omitempty"`
	NeedsLogin       bool   `json:"needsLogin,omitempty"`
	AuthURL          string `json:"authUrl,omitempty"`
	FunnelNotEnabled bool   `json:"funnelNotEnabled,omitempty"`
	EnableURL        string `json:"enableUrl,omitempty"`
	Error            string `json:"error,omitempty"`
}

// CheckResult holds the result of checking Tailscale installation.
type CheckResult struct {
	Installed       bool   `json:"installed"`
	LoggedIn        bool   `json:"loggedIn"`
	Platform        string `json:"platform"`
	DaemonRunning   bool   `json:"daemonRunning"`
	HasSudoPassword bool   `json:"hasCachedPassword"`
	BrewAvailable   bool   `json:"brewAvailable"`
}

// NeedSudoPassword returns true if the platform/install method requires a sudo
// password for TUN daemon mode. macOS with brew can install without sudo, but
// still needs sudo for TUN daemon.
func (m *Manager) NeedSudoPassword() bool {
	if runtime.GOOS == "windows" {
		return false
	}
	// macOS with brew: install doesn't need sudo, but TUN daemon still does.
	// We still require password for proper TUN funnel support.
	return true
}

// Enable starts the Tailscale funnel. The flow:
// 1. Validate sudo password (if TUN mode needed)
// 2. Install tailscale if not installed (brew/pkg/none)
// 3. Start daemon with sudo password
// 4. Check login → if not logged in, return needsLogin + authUrl
// 5. Stop existing funnel
// 6. Start funnel → get *.ts.net URL
// 7. If funnelNotEnabled, return enableUrl
// 8. Provision TLS cert
// 9. Wait for health (non-fatal timeout)
func (m *Manager) Enable(sudoPassword string, settingsUpdate func(tunnelURL string)) (*EnableResult, error) {
	m.mu.Lock()
	if m.spawnInProgress {
		m.mu.Unlock()
		return nil, fmt.Errorf("tailscale enable already in progress")
	}
	m.spawnInProgress = true
	m.cancelToken = make(chan struct{})
	token := m.cancelToken
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.spawnInProgress = false
		m.mu.Unlock()
	}()

	m.log.Info("[Tailscale] enable start", "port", m.localPort)

	// Validate sudo password up-front so we fail fast with a clear error
	// instead of silently proceeding when the password is wrong.
	// macOS+brew can install without sudo, but TUN daemon still needs it.
	if sudoPassword != "" {
		if err := ValidateSudoPassword(sudoPassword); err != nil {
			m.log.Error("[Tailscale] sudo password validation failed", "error", err)
			return &EnableResult{
				Success: false,
				Error:   fmt.Sprintf("sudo password validation failed: %s", err.Error()),
			}, nil
		}
	}

	// Install tailscale if not already installed.
	if !IsInstalled(m.dataDir) {
		m.log.Info("[Tailscale] not installed, installing...")
		if err := InstallTailscale(m.dataDir, sudoPassword, func(msg string) {
			m.log.Info("[Tailscale] install", "msg", msg)
		}); err != nil {
			m.log.Error("[Tailscale] install failed", "error", err)
			return &EnableResult{
				Success: false,
				Error:   fmt.Sprintf("tailscale install failed: %s", err.Error()),
			}, nil
		}
	}

	// Start daemon.
	if err := StartDaemon(m.dataDir, sudoPassword, m.log); err != nil {
		m.log.Error("[Tailscale] daemon start failed", "error", err)
		return nil, err
	}
	m.log.Info("[Tailscale] daemon ready")

	select {
	case <-token:
		return nil, fmt.Errorf("tailscale canceled")
	default:
	}

	// Get or generate short ID for hostname.
	existing := tunnel.LoadState(m.dataDir)
	shortID := ""
	if existing != nil && existing.ShortID != "" {
		shortID = existing.ShortID
	} else {
		shortID = tunnel.GenerateShortID()
	}

	// Check login state.
	loggedIn := IsLoggedIn(m.dataDir)
	m.log.Info("[Tailscale] login check", "loggedIn", loggedIn)

	if !loggedIn {
		loginResult, err := StartLogin(m.dataDir, shortID)
		if err != nil {
			return nil, err
		}
		if loginResult.AuthURL != "" {
			m.log.Info("[Tailscale] needs login", "authUrl", loginResult.AuthURL)
			return &EnableResult{
				Success:    false,
				NeedsLogin: true,
				AuthURL:    loginResult.AuthURL,
			}, nil
		}
	}

	select {
	case <-token:
		return nil, fmt.Errorf("tailscale canceled")
	default:
	}

	// Stop existing funnel.
	StopFunnel(m.dataDir)

	// Start funnel.
	m.log.Info("[Tailscale] starting funnel")
	funnelResult, err := StartFunnel(m.dataDir, m.localPort)
	if err != nil {
		m.log.Error("[Tailscale] funnel error", "error", err)
		// Auto-retry via login if daemon state is wrong.
		if isLoginError(err) {
			loginResult, loginErr := StartLogin(m.dataDir, shortID)
			if loginErr == nil && loginResult.AuthURL != "" {
				return &EnableResult{
					Success:    false,
					NeedsLogin: true,
					AuthURL:    loginResult.AuthURL,
				}, nil
			}
		}
		return nil, err
	}

	select {
	case <-token:
		return nil, fmt.Errorf("tailscale canceled")
	default:
	}

	if funnelResult.FunnelNotEnabled {
		m.log.Info("[Tailscale] funnel not enabled", "enableUrl", funnelResult.EnableURL)
		return &EnableResult{
			Success:          false,
			FunnelNotEnabled: true,
			EnableURL:        funnelResult.EnableURL,
		}, nil
	}

	// Strict probe.
	if !IsLoggedIn(m.dataDir) || !isFunnelRunning(m.dataDir) {
		m.log.Error("[Tailscale] strict probe failed")
		StopFunnel(m.dataDir)
		return &EnableResult{
			Success: false,
			Error:   "Tailscale not connected. Device may have been removed. Please re-login.",
		}, nil
	}

	// Update settings.
	if settingsUpdate != nil {
		settingsUpdate(funnelResult.TunnelURL)
	}
	m.log.Info("[Tailscale] funnel up", "url", funnelResult.TunnelURL)

	// Provision TLS cert (best-effort).
	if parsed, err := url.Parse(funnelResult.TunnelURL); err == nil {
		ProvisionCert(m.dataDir, parsed.Hostname(), m.log)
	}

	// Verify health (non-fatal timeout).
	reachable := false
	if err := tunnel.WaitForHealth(funnelResult.TunnelURL, tunnel.TailscaleHealthConfig, token); err != nil {
		m.log.Warn("[Tailscale] health check timed out", "error", err)
	} else {
		reachable = true
	}

	m.log.Info("[Tailscale] enable success", "reachable", reachable)
	return &EnableResult{
		Success:   true,
		TunnelURL: funnelResult.TunnelURL,
	}, nil
}

// Disable stops the Tailscale funnel.
func (m *Manager) Disable(settingsUpdate func()) {
	m.log.Info("[Tailscale] disable")

	m.mu.Lock()
	if m.cancelToken != nil {
		close(m.cancelToken)
		m.cancelToken = nil
	}
	m.spawnInProgress = false
	m.mu.Unlock()

	StopFunnel(m.dataDir)

	if settingsUpdate != nil {
		settingsUpdate()
	}
}

// Status returns the current Tailscale status.
func (m *Manager) Status() TailscaleStatus {
	installed := IsInstalled(m.dataDir)
	loggedIn := false
	running := false
	tunnelURL := ""

	if installed {
		loggedIn = IsLoggedIn(m.dataDir)
		if loggedIn {
			running = isFunnelRunning(m.dataDir)
			if running {
				tunnelURL = GetFunnelURL(m.dataDir)
			}
		}
	}

	return TailscaleStatus{
		Enabled:   running,
		TunnelURL: tunnelURL,
		Running:   running,
		LoggedIn:  loggedIn,
		Installed: installed,
		Platform:  runtime.GOOS,
	}
}

// Check returns detailed installation and state info.
func (m *Manager) Check(sudoPassword string) CheckResult {
	installed := IsInstalled(m.dataDir)
	daemonRunning := false
	loggedIn := false

	if installed {
		daemonRunning = isDaemonResponsive(m.dataDir)
		if daemonRunning {
			loggedIn = IsLoggedIn(m.dataDir)
		}
	}

	return CheckResult{
		Installed:       installed,
		LoggedIn:        loggedIn,
		Platform:        runtime.GOOS,
		DaemonRunning:   daemonRunning,
		HasSudoPassword: sudoPassword != "",
		BrewAvailable:   runtime.GOOS == "darwin" && HasBrew(),
	}
}

// isFunnelRunning checks if the funnel is active by querying funnel status.
func isFunnelRunning(dataDir string) bool {
	bin := FindBinary(dataDir)
	if bin == "" {
		return false
	}
	out, err := exec.Command(bin, tsArgs(dataDir, "funnel", "status", "--json")...).Output()
	if err != nil {
		return false
	}
	// Check if any funnel entries exist.
	return len(out) > 2 && string(out) != "{}"
}

func isLoginError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "NoState") || strings.Contains(msg, "unexpected state") ||
		strings.Contains(msg, "not logged in") || strings.Contains(msg, "Logged out") ||
		strings.Contains(msg, "NeedsLogin")
}
