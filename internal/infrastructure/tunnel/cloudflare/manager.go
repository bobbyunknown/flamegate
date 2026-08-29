package cloudflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel"
)

// DefaultWorkerURL is the default URL registration worker.
const DefaultWorkerURL = "https://abc-tunnel.us"

// Manager orchestrates the Cloudflare quick tunnel lifecycle.
type Manager struct {
	dataDir   string
	localPort int
	log       *logrus.Logger
	workerURL string

	mu              sync.Mutex
	cancelToken     chan struct{}
	spawnInProgress bool
}

// NewManager creates a new Cloudflare tunnel manager.
func NewManager(dataDir string, localPort int, log *logrus.Logger) *Manager {
	workerURL := os.Getenv("TUNNEL_WORKER_URL")
	if workerURL == "" {
		workerURL = DefaultWorkerURL
	}
	return &Manager{
		dataDir:   dataDir,
		localPort: localPort,
		log:       log,
		workerURL: workerURL,
	}
}

// TunnelStatus holds the current tunnel state.
type TunnelStatus struct {
	Enabled         bool   `json:"enabled"`
	SettingsEnabled bool   `json:"settingsEnabled"`
	TunnelURL       string `json:"tunnelUrl"`
	ShortID         string `json:"shortId"`
	PublicURL       string `json:"publicUrl"`
	Running         bool   `json:"running"`
}

// EnableResult holds the result of enabling a tunnel.
type EnableResult struct {
	Success        bool   `json:"success"`
	TunnelURL      string `json:"tunnelUrl"`
	ShortID        string `json:"shortId"`
	PublicURL      string `json:"publicUrl"`
	AlreadyRunning bool   `json:"alreadyRunning,omitempty"`
}

// registerTunnelUrl registers the tunnel URL with the worker to get a short
// public URL.
func (m *Manager) registerTunnelURL(shortID, tunnelURL string) error {
	payload, _ := json.Marshal(map[string]string{
		"shortId":   shortID,
		"tunnelUrl": tunnelURL,
	})
	resp, err := http.Post(
		m.workerURL+"/api/tunnel/register",
		"application/json",
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("register tunnel URL: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("register tunnel URL: status %d", resp.StatusCode)
	}
	return nil
}

// Enable starts a Cloudflare quick tunnel. The flow:
// 1. Check if already running + health → reuse
// 2. Kill existing cloudflared
// 3. Spawn quick tunnel → get trycloudflare.com URL
// 4. Register with worker → get short public URL
// 5. Save state, update settings
// 6. Wait for health on public URL
func (m *Manager) Enable(settingsUpdate func(tunnelURL string)) (*EnableResult, error) {
	m.mu.Lock()
	if m.spawnInProgress {
		m.mu.Unlock()
		return nil, fmt.Errorf("tunnel enable already in progress")
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

	m.log.Info("[Tunnel] enable start", "port", m.localPort)

	// Check if already running.
	if IsCloudflaredRunning(m.dataDir) {
		existing := tunnel.LoadState(m.dataDir)
		if existing != nil && existing.TunnelURL != "" && existing.ShortID != "" {
			publicURL := fmt.Sprintf("https://r%s.abc-tunnel.us", existing.ShortID)
			directOK := tunnel.ProbeURLAlive(existing.TunnelURL, tunnel.CloudflareHealthConfig)
			publicOK := tunnel.ProbeURLAlive(publicURL, tunnel.CloudflareHealthConfig)
			if directOK && publicOK {
				m.log.Info("[Tunnel] already running, reusing", "url", existing.TunnelURL)
				return &EnableResult{
					Success:        true,
					TunnelURL:      existing.TunnelURL,
					ShortID:        existing.ShortID,
					PublicURL:      publicURL,
					AlreadyRunning: true,
				}, nil
			}
			m.log.Info("[Tunnel] stale, respawning", "direct", directOK, "public", publicOK)
		}
	}

	// Kill existing.
	KillCloudflared(m.dataDir, m.localPort)
	m.log.Info("[Tunnel] killed existing cloudflared")

	select {
	case <-token:
		return nil, fmt.Errorf("tunnel canceled")
	default:
	}

	// Get or generate short ID.
	existing := tunnel.LoadState(m.dataDir)
	shortID := ""
	if existing != nil && existing.ShortID != "" {
		shortID = existing.ShortID
	} else {
		shortID = tunnel.GenerateShortID()
	}

	// Spawn quick tunnel.
	result, err := SpawnQuickTunnel(m.dataDir, m.localPort, m.log)
	if err != nil {
		m.log.Error("[Tunnel] spawn failed", "error", err)
		return nil, err
	}

	select {
	case <-token:
		result.Cmd.Process.Kill()
		return nil, fmt.Errorf("tunnel canceled")
	default:
	}

	tunnelURL := result.TunnelURL
	publicURL := fmt.Sprintf("https://r%s.abc-tunnel.us", shortID)

	// Register with worker.
	if err := m.registerTunnelURL(shortID, tunnelURL); err != nil {
		m.log.Warn("[Tunnel] worker registration failed (non-fatal)", "error", err)
	}

	// Save state.
	_ = tunnel.SaveState(m.dataDir, &tunnel.TunnelState{
		ShortID:   shortID,
		TunnelURL: tunnelURL,
	})

	// Update settings.
	if settingsUpdate != nil {
		settingsUpdate(tunnelURL)
	}

	m.log.Info("[Tunnel] registered", "shortId", shortID, "publicUrl", publicURL)

	// Wait for health on public URL.
	if err := tunnel.WaitForHealth(publicURL, tunnel.CloudflareHealthConfig, token); err != nil {
		m.log.Warn("[Tunnel] public URL health check failed", "error", err)
	} else {
		m.log.Info("[Tunnel] public URL healthy")
	}

	m.log.Info("[Tunnel] enable success")
	return &EnableResult{
		Success:   true,
		TunnelURL: tunnelURL,
		ShortID:   shortID,
		PublicURL: publicURL,
	}, nil
}

// Disable stops the Cloudflare tunnel.
func (m *Manager) Disable(settingsUpdate func()) {
	m.log.Info("[Tunnel] disable")

	m.mu.Lock()
	if m.cancelToken != nil {
		close(m.cancelToken)
		m.cancelToken = nil
	}
	m.spawnInProgress = false
	m.mu.Unlock()

	KillCloudflared(m.dataDir, m.localPort)

	// Preserve short ID, clear tunnel URL.
	existing := tunnel.LoadState(m.dataDir)
	if existing != nil {
		_ = tunnel.SaveState(m.dataDir, &tunnel.TunnelState{
			ShortID:   existing.ShortID,
			TunnelURL: "",
		})
	}

	if settingsUpdate != nil {
		settingsUpdate()
	}
}

// Status returns the current tunnel status.
func (m *Manager) Status() TunnelStatus {
	state := tunnel.LoadState(m.dataDir)
	shortID := ""
	publicURL := ""
	tunnelURL := ""
	if state != nil {
		shortID = state.ShortID
		tunnelURL = state.TunnelURL
		if shortID != "" {
			publicURL = fmt.Sprintf("https://r%s.abc-tunnel.us", shortID)
		}
	}
	running := IsCloudflaredRunning(m.dataDir)
	return TunnelStatus{
		Enabled:   running && tunnelURL != "",
		TunnelURL: tunnelURL,
		ShortID:   shortID,
		PublicURL: publicURL,
		Running:   running,
	}
}
