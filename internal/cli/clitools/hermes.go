package clitools

import (
	"os"

	"gopkg.in/yaml.v3"
)

// HermesTool auto-configures Hermes Agent (~/.hermes/config.yaml + .env).
type HermesTool struct{}

func (t *HermesTool) ID() string   { return "hermes" }
func (t *HermesTool) Name() string { return "Hermes Agent" }

func (t *HermesTool) configPath(homeDir string) string {
	return expandHome(homeDir, "~/.hermes/config.yaml")
}
func (t *HermesTool) envPath(homeDir string) string {
	return expandHome(homeDir, "~/.hermes/.env")
}

func (t *HermesTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	raw, err := readString(path)
	if err != nil {
		return true, false, path, err
	}
	configured := containsLocalhost(raw) && containsFlameGate(raw)
	return true, configured, path, nil
}

func (t *HermesTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	configPath := t.configPath(homeDir)
	envP := t.envPath(homeDir)

	model := "gpt-4o"
	if len(models) > 0 {
		model = models[0]
	}

	cfg := make(map[string]any)
	if raw, err := readString(configPath); err == nil {
		_ = yaml.Unmarshal([]byte(raw), &cfg)
	}

	cfg["model"] = map[string]any{
		"default":  model,
		"provider": "custom",
		"base_url": ensureSuffix(baseURL, "/v1"),
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := writeString(configPath, string(data)); err != nil {
		return err
	}

	return writeString(envP, "OPENAI_API_KEY="+apiKey+"\n")
}

func (t *HermesTool) Remove(homeDir string) error {
	configPath := t.configPath(homeDir)
	envP := t.envPath(homeDir)

	if fileExists(configPath) {
		raw, err := readString(configPath)
		if err == nil {
			cfg := make(map[string]any)
			_ = yaml.Unmarshal([]byte(raw), &cfg)
			if model, ok := cfg["model"].(map[string]any); ok {
				if provider, ok := model["provider"].(string); ok && provider == "custom" {
					delete(cfg, "model")
				}
			}
			if len(cfg) == 0 {
				_ = os.Remove(configPath)
			} else {
				data, _ := yaml.Marshal(cfg)
				_ = writeString(configPath, string(data))
			}
		}
	}

	if fileExists(envP) {
		_ = os.Remove(envP)
	}
	return nil
}
