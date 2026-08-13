package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel/tailscale"
)

// adminTunnelStatus returns the combined tunnel + tailscale + download status.

// adminTunnelEnable starts the Cloudflare quick tunnel.

// adminTunnelDisable stops the Cloudflare tunnel.

// adminTailscaleCheck returns installation and state info for Tailscale.

// adminTailscaleEnable starts the Tailscale funnel.

// adminTailscaleDisable stops the Tailscale funnel.

// adminTailscaleInstall handles Tailscale installation with SSE streaming
// progress events.
func (s *Handler) adminTailscaleInstall(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	var body struct {
		SudoPassword string `json:"sudoPassword"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck // best-effort decode
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sendEvent := func(event string, data any) {
		d, _ := json.Marshal(data)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, d)
		flusher.Flush()
	}

	onProgress := func(msg string) {
		sendEvent("progress", map[string]string{"message": msg})
	}

	// Validate sudo password before install for paths that require it
	// (Linux, macOS without brew). macOS+brew doesn't need sudo for install
	// but still needs it for TUN daemon later.
	needsSudoForInstall := !tailscale.HasBrew() || runtime.GOOS != "darwin"
	if needsSudoForInstall {
		if err := tailscale.ValidateSudoPassword(body.SudoPassword); err != nil {
			sendEvent("error", map[string]string{"error": fmt.Sprintf("sudo password validation failed: %s", err.Error())})
			return
		}
	}

	err := tailscale.InstallTailscale(s.dataDir, body.SudoPassword, onProgress)
	if err != nil {
		errMsg := err.Error()
		if contains(errMsg, "incorrect password") || contains(errMsg, "Sorry") {
			errMsg = "Wrong sudo password"
		}
		sendEvent("error", map[string]string{"error": errMsg})
		return
	}

	sendEvent("done", map[string]any{"success": true})
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
