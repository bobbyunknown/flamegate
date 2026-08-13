package clitools

import (
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

// JcodeTool auto-configures jcode (~/.jcode/config.toml + env file).
type JcodeTool struct{}

func (t *JcodeTool) ID() string   { return "jcode" }
func (t *JcodeTool) Name() string { return "jcode" }

func (t *JcodeTool) configPath(homeDir string) string {
	return expandHome(homeDir, "~/.jcode/config.toml")
}
func (t *JcodeTool) envPath(homeDir string) string {
	return expandHome(homeDir, "~/.config/jcode/provider-flamegate.env")
}

func (t *JcodeTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	raw, err := readString(path)
	if err != nil {
		return true, false, path, err
	}
	configured := containsFlameGate(raw)
	return true, configured, path, nil
}

func (t *JcodeTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	configPath := t.configPath(homeDir)
	envP := t.envPath(homeDir)

	model := "gpt-4o"
	if len(models) > 0 {
		model = models[0]
	}

	cfg := make(map[string]any)
	if raw, err := readString(configPath); err == nil {
		_ = toml.Unmarshal([]byte(raw), &cfg)
	}

	providers, _ := cfg["providers"].(map[string]any)
	if providers == nil {
		providers = make(map[string]any)
	}
	providers["flamegate"] = map[string]any{
		"type":          "openai-compatible",
		"base_url":      ensureSuffix(baseURL, "/v1"),
		"auth":          "bearer",
		"api_key_env":   "JCODE_FLAMEGATE_API_KEY",
		"env_file":      envP,
		"default_model": model,
	}
	cfg["providers"] = providers

	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := writeString(configPath, string(data)); err != nil {
		return err
	}

	return writeString(envP, "JCODE_FLAMEGATE_API_KEY="+apiKey+"\n")
}

func (t *JcodeTool) Remove(homeDir string) error {
	configPath := t.configPath(homeDir)
	envP := t.envPath(homeDir)

	if fileExists(configPath) {
		raw, err := readString(configPath)
		if err == nil {
			cfg := make(map[string]any)
			_ = toml.Unmarshal([]byte(raw), &cfg)
			if providers, ok := cfg["providers"].(map[string]any); ok {
				delete(providers, "flamegate")
				if len(providers) == 0 {
					delete(cfg, "providers")
				}
			}
			if len(cfg) == 0 {
				_ = os.Remove(configPath)
			} else {
				data, _ := toml.Marshal(cfg)
				_ = writeString(configPath, string(data))
			}
		}
	}

	if fileExists(envP) {
		_ = os.Remove(envP)
	}
	return nil
}
