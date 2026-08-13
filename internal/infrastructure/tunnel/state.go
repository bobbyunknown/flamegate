// Package tunnel implements secure tunnel management for FlameGate, providing
// the Cloudflare quick tunnel and Tailscale funnel features.
package tunnel

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ShortIDLength is the length of the random short ID used for public URLs.
const ShortIDLength = 6

// shortIDChars excludes ambiguous characters (0, O, l, 1, I).
const shortIDChars = "abcdefghijklmnpqrstuvwxyz23456789"

// TunnelState is the persisted tunnel state, written to data_dir/tunnel/state.json.
type TunnelState struct {
	ShortID   string `json:"shortId"`
	TunnelURL string `json:"tunnelUrl"`
}

// TunnelDir returns the path to the tunnel data directory.
func TunnelDir(dataDir string) string {
	return filepath.Join(dataDir, "tunnel")
}

// EnsureTunnelDir creates the tunnel data directory if it doesn't exist.
func EnsureTunnelDir(dataDir string) error {
	return os.MkdirAll(TunnelDir(dataDir), 0o700)
}

// StateFile returns the path to the tunnel state file.
func StateFile(dataDir string) string {
	return filepath.Join(TunnelDir(dataDir), "state.json")
}

// LoadState reads and parses the tunnel state file. Returns nil if the file
// doesn't exist or is corrupt.
func LoadState(dataDir string) *TunnelState {
	data, err := os.ReadFile(StateFile(dataDir))
	if err != nil {
		return nil //nolint:nilerr // best-effort loader
	}
	var s TunnelState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil //nolint:nilerr // best-effort loader
	}
	return &s
}

// SaveState writes the tunnel state to disk as pretty-printed JSON.
func SaveState(dataDir string, state *TunnelState) error {
	if err := EnsureTunnelDir(dataDir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StateFile(dataDir), data, 0o600)
}

// ClearState removes the tunnel state file.
func ClearState(dataDir string) {
	_ = os.Remove(StateFile(dataDir))
}

// GenerateShortID creates a random 6-character identifier from alphanumeric
// characters (excluding ambiguous 0, O, l, 1, I).
func GenerateShortID() string {
	b := make([]byte, ShortIDLength)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = shortIDChars[int(b[i])%len(shortIDChars)]
	}
	return string(b)
}

// PIDFile returns the path to the cloudflared PID file.
func PIDFile(dataDir string) string {
	return filepath.Join(TunnelDir(dataDir), "cloudflared.pid")
}

// SavePID writes a PID to the PID file.
func SavePID(dataDir string, pid int) error {
	if err := EnsureTunnelDir(dataDir); err != nil {
		return err
	}
	return os.WriteFile(PIDFile(dataDir), []byte(fmt.Sprintf("%d", pid)), 0o600)
}

// LoadPID reads the PID from the PID file. Returns 0 if not found.
func LoadPID(dataDir string) int {
	data, err := os.ReadFile(PIDFile(dataDir))
	if err != nil {
		return 0
	}
	var pid int
	if err := json.Unmarshal(data, &pid); err != nil {
		// Try parsing as plain text.
		n := 0
		for _, c := range string(data) {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		return n
	}
	return pid
}

// ClearPID removes the PID file.
func ClearPID(dataDir string) {
	_ = os.Remove(PIDFile(dataDir))
}
