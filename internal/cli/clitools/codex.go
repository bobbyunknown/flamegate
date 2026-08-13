package clitools

import (
	"os"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// CodexTool auto-configures Codex CLI (~/.codex/config.toml + auth.json).
type CodexTool struct{}

func (t *CodexTool) ID() string   { return "codex" }
func (t *CodexTool) Name() string { return "Codex CLI" }

func (t *CodexTool) configPath(homeDir string) string {
	return expandHome(homeDir, "~/.codex/config.toml")
}
func (t *CodexTool) authPath(homeDir string) string {
	return expandHome(homeDir, "~/.codex/auth.json")
}

func (t *CodexTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	raw, err := readString(path)
	if err != nil {
		return true, false, path, err
	}
	configured := strings.Contains(raw, "model_provider") && strings.Contains(raw, "flamegate")
	return true, configured, path, nil
}

func (t *CodexTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	configPath := t.configPath(homeDir)
	authPath := t.authPath(homeDir)

	model := "gpt-4o"
	if len(models) > 0 {
		model = models[0]
	}

	// Parse existing config or start fresh.
	cfg := make(map[string]any)
	if raw, err := readString(configPath); err == nil {
		_ = toml.Unmarshal([]byte(raw), &cfg)
	}

	cfg["model"] = model
	cfg["model_provider"] = "flamegate"

	providers, _ := cfg["model_providers"].(map[string]any)
	if providers == nil {
		providers = make(map[string]any)
	}
	providers["flamegate"] = map[string]any{
		"name":     "FlameGate",
		"base_url": ensureSuffix(baseURL, "/v1"),
		"wire_api": "responses",
	}
	cfg["model_providers"] = providers

	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := writeString(configPath, string(data)); err != nil {
		return err
	}

	// Write auth.json.
	auth := map[string]string{
		"OPENAI_API_KEY": apiKey,
		"auth_mode":      "apikey",
	}
	return writeJSON(authPath, auth)
}

func (t *CodexTool) Remove(homeDir string) error {
	configPath := t.configPath(homeDir)
	authPath := t.authPath(homeDir)

	if fileExists(configPath) {
		raw, err := readString(configPath)
		if err == nil {
			cfg := make(map[string]any)
			_ = toml.Unmarshal([]byte(raw), &cfg)
			if mp, ok := cfg["model_provider"].(string); ok && mp == "flamegate" {
				delete(cfg, "model_provider")
				delete(cfg, "model")
			}
			if providers, ok := cfg["model_providers"].(map[string]any); ok {
				delete(providers, "flamegate")
				if len(providers) == 0 {
					delete(cfg, "model_providers")
				}
			}
			if len(cfg) == 0 {
				_ = os.Remove(configPath)
			} else {
				data, _ := toml.Marshal(cfg)
				writeString(configPath, string(data)) //nolint:errcheck // best-effort
			}
		}
	}

	if fileExists(authPath) {
		_ = os.Remove(authPath)
	}
	return nil
}
