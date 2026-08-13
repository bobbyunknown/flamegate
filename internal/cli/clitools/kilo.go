package clitools

import (
	"os"
)

// KiloTool auto-configures Kilo Code (~/.local/share/kilo/auth.json).
type KiloTool struct{}

func (t *KiloTool) ID() string   { return "kilo" }
func (t *KiloTool) Name() string { return "Kilo Code" }

func (t *KiloTool) configPath(homeDir string) string {
	return expandHome(homeDir, "~/.local/share/kilo/auth.json")
}

func (t *KiloTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	var cfg map[string]any
	if err := readJSON(path, &cfg); err != nil {
		return true, false, path, err
	}
	if oc, ok := cfg["openai-compatible"].(map[string]any); ok {
		if base, ok := oc["baseUrl"].(string); ok && containsLocalhost(base) {
			return true, true, path, nil
		}
	}
	return true, false, path, nil
}

func (t *KiloTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	path := t.configPath(homeDir)
	cfg := make(map[string]any)
	_ = readJSON(path, &cfg)

	model := "gpt-4o"
	if len(models) > 0 {
		model = models[0]
	}

	cfg["openai-compatible"] = map[string]any{
		"type":    "api-key",
		"apiKey":  apiKey,
		"baseUrl": ensureSuffix(baseURL, "/v1"),
		"model":   model,
	}
	return writeJSON(path, cfg)
}

func (t *KiloTool) Remove(homeDir string) error {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return nil
	}
	var cfg map[string]any
	if err := readJSON(path, &cfg); err != nil {
		return err
	}
	delete(cfg, "openai-compatible")
	if len(cfg) == 0 {
		return os.Remove(path)
	}
	return writeJSON(path, cfg)
}
