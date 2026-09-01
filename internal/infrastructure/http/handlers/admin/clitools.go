package admin

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/bobbyunknown/flamegate/internal/cli/clitools"
)

// handleCLITools returns the status and config snippets for all supported CLI
// tools. The frontend uses the snippet fields for copy-to-clipboard; the status
// fields drive the installed/configured badges.
func (s *Handler) HandleCLITools(w http.ResponseWriter, r *http.Request) {
	statuses := s.cliTools.DetectAll(s.cliToolHome)
	baseURL := s.publicProxyBaseURL(r)
	model := r.URL.Query().Get("model")

	// Build a lookup so we can merge snippet metadata with live status.
	snippets := generateSnippets(baseURL, model, "")

	// Merge: for each snippet entry, find the matching status and combine.
	type toolResp struct {
		cliToolSnippet
		Installed  bool   `json:"installed"`
		Configured bool   `json:"configured"`
		ConfigPath string `json:"config_path"`
	}
	statusMap := make(map[string]clitools.Status, len(statuses))
	for _, st := range statuses {
		statusMap[st.ID] = st
	}
	tools := make([]toolResp, 0, len(snippets))
	for _, sn := range snippets {
		tr := toolResp{cliToolSnippet: sn}
		if st, ok := statusMap[sn.ID]; ok {
			tr.Installed = st.Installed
			tr.Configured = st.Configured
			tr.ConfigPath = st.ConfigPath
		}
		tools = append(tools, tr)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"base_url": baseURL,
		"model":    model,
		"tools":    tools,
	})
}

// handleCLIToolConfigure writes FlameGate config into a specific tool.
func (s *Handler) HandleCLIToolConfigure(w http.ResponseWriter, r *http.Request) {
	toolID := chi.URLParam(r, "toolId")
	tool := s.cliTools.Get(toolID)
	if tool == nil {
		WriteError(w, http.StatusNotFound, "unknown tool: "+toolID)
		return
	}

	var body struct {
		BaseURL string   `json:"base_url"`
		APIKey  string   `json:"api_key"`
		Models  []string `json:"models"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.BaseURL == "" {
		WriteError(w, http.StatusBadRequest, "base_url is required")
		return
	}
	if body.APIKey == "" {
		WriteError(w, http.StatusBadRequest, "api_key is required")
		return
	}

	if err := tool.Configure(s.cliToolHome, body.BaseURL, body.APIKey, body.Models); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleCLIToolRemove strips FlameGate config from a specific tool.
func (s *Handler) HandleCLIToolRemove(w http.ResponseWriter, r *http.Request) {
	toolID := chi.URLParam(r, "toolId")
	tool := s.cliTools.Get(toolID)
	if tool == nil {
		WriteError(w, http.StatusNotFound, "unknown tool: "+toolID)
		return
	}

	if err := tool.Remove(s.cliToolHome); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// publicBaseURL derives the externally usable base URL from the request. It
// honors a forwarded host/proto when present (reverse proxy), else falls back
// to the configured listen address.
func (s *Handler) publicBaseURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	if host == "" {
		host = s.cfg.Addr()
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

// publicProxyBaseURL derives the externally usable base URL for the LLM /v1 proxy.
// If a dedicated proxy port is configured, it uses that port.
func (s *Handler) publicProxyBaseURL(r *http.Request) string {
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	if host == "" {
		host = s.cfg.Addr()
	}

	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
	}

	proxyPort := s.cfg.Server.Port
	if s.cfg.Server.ProxyPort > 0 {
		proxyPort = s.cfg.Server.ProxyPort
	}

	targetHost := net.JoinHostPort(hostname, strconv.Itoa(proxyPort))
	return fmt.Sprintf("%s://%s", scheme, targetHost)
}

// mountCLITools registers the CLI tool auto-config endpoints.
func (s *Handler) MountCLITools(r chi.Router) {
	r.Get("/cli-tools", s.HandleCLITools)
	r.Post("/cli-tools/{toolId}/configure", s.HandleCLIToolConfigure)
	r.Post("/cli-tools/{toolId}/remove", s.HandleCLIToolRemove)
}
