package clitools

import (
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// DeepSeekTool auto-configures DeepSeek TUI (~/.deepseek/config.toml).
type DeepSeekTool struct{}

func (t *DeepSeekTool) ID() string   { return "deepseek" }
func (t *DeepSeekTool) Name() string { return "DeepSeek TUI" }

func (t *DeepSeekTool) configPath(homeDir string) string {
	return expandHome(homeDir, "~/.deepseek/config.toml")
}

func (t *DeepSeekTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	raw, err := readString(path)
	if err != nil {
		return true, false, path, err
	}
	configured := strings.Contains(raw, "provider") && containsLocalhost(raw)
	return true, configured, path, nil
}

func (t *DeepSeekTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	path := t.configPath(homeDir)

	model := "deepseek-chat"
	if len(models) > 0 {
		model = models[0]
	}

	cfg := map[string]any{
		"provider": "openai",
		"providers": map[string]any{
			"openai": map[string]any{
				"base_url": ensureSuffix(baseURL, "/v1"),
				"api_key":  apiKey,
				"model":    model,
			},
		},
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeString(path, string(data))
}

func (t *DeepSeekTool) Remove(homeDir string) error {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return nil
	}

	// Reset to default: just provider = "deepseek", no custom providers.
	cfg := map[string]any{
		"provider": "deepseek",
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return writeString(path, string(data))
}
