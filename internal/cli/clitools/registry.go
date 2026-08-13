// Package clitools implements auto-configuration for coding CLI tools. Each
// tool knows how to detect its installation status, write FlameGate-specific
// config keys into the tool's native config file, and remove them again.
//
// The common strategy is merge-not-overwrite: existing settings are preserved;
// only FlameGate-specific keys are added or removed.
package clitools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Tool describes a CLI tool that can be auto-configured to use FlameGate.
type Tool interface {
	// ID returns the tool's short identifier (e.g. "claude", "codex").
	ID() string
	// Name returns the human-readable display name.
	Name() string
	// DetectStatus checks whether the tool is installed and whether FlameGate
	// is already configured. configPath is the path that was checked.
	DetectStatus(homeDir string) (installed, configured bool, configPath string, err error)
	// Configure writes FlameGate settings into the tool's config file(s).
	Configure(homeDir, baseURL, apiKey string, models []string) error
	// Remove strips FlameGate settings from the tool's config file(s).
	Remove(homeDir string) error
}

// Status holds the result of DetectStatus for one tool.
type Status struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Installed  bool   `json:"installed"`
	Configured bool   `json:"configured"`
	ConfigPath string `json:"config_path"`
}

// Registry holds all known CLI tools.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry builds a registry with all built-in tool implementations.
func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	for _, t := range []Tool{
		&ClaudeTool{},
		&CodexTool{},
		&ClineTool{},
		&CopilotTool{},
		&DroidTool{},
		&OpenClawTool{},
		&OpenCodeTool{},
		&KiloTool{},
		&HermesTool{},
		&DeepSeekTool{},
		&JcodeTool{},
	} {
		r.tools[t.ID()] = t
	}
	return r
}

// Get returns a tool by id, or nil if unknown.
func (r *Registry) Get(id string) Tool { return r.tools[id] }

// All returns all registered tools.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// DetectAll returns the status of every registered tool.
func (r *Registry) DetectAll(homeDir string) []Status {
	out := make([]Status, 0, len(r.tools))
	for _, t := range r.All() {
		inst, conf, path, _ := t.DetectStatus(homeDir)
		out = append(out, Status{
			ID: t.ID(), Name: t.Name(),
			Installed: inst, Configured: conf, ConfigPath: path,
		})
	}
	return out
}

// ---- common helpers --------------------------------------------------------

// expandHome resolves ~ to homeDir.
func expandHome(homeDir, path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readJSON reads a JSON file into v. Returns os.ErrNotExist if missing.
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// writeJSON writes v as indented JSON to path, creating dirs as needed.
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

// readString reads a file into a string.
func readString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// writeString writes content to path, creating dirs as needed.
func writeString(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

// ensureSuffix ensures baseURL ends with suffix.
func ensureSuffix(baseURL, suffix string) string {
	if !strings.HasSuffix(baseURL, suffix) {
		return strings.TrimRight(baseURL, "/") + suffix
	}
	return baseURL
}

// stripSuffix removes suffix from the end of baseURL if present.
func stripSuffix(baseURL, suffix string) string {
	return strings.TrimSuffix(strings.TrimRight(baseURL, "/"), suffix)
}

// platformConfigPath returns the platform-specific VS Code config path.
func platformConfigPath(homeDir, suffix string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Code", "User", suffix)
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "Code", "User", suffix)
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "Code", "User", suffix)
	default: // linux
		return filepath.Join(homeDir, ".config", "Code", "User", suffix)
	}
}
