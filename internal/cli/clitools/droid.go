package clitools

import (
	"fmt"
	"os"
	"strings"
)

// DroidTool auto-configures Factory Droid (~/.factory/settings.json).
type DroidTool struct{}

func (t *DroidTool) ID() string   { return "droid" }
func (t *DroidTool) Name() string { return "Factory Droid" }

func (t *DroidTool) configPath(homeDir string) string {
	return expandHome(homeDir, "~/.factory/settings.json")
}

func (t *DroidTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	raw, err := readString(path)
	if err != nil {
		return true, false, path, err
	}
	configured := strings.Contains(raw, "custom:FlameGate")
	return true, configured, path, nil
}

func (t *DroidTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	path := t.configPath(homeDir)
	cfg := make(map[string]any)
	_ = readJSON(path, &cfg)

	customModels := make([]map[string]any, len(models))
	for i, m := range models {
		customModels[i] = map[string]any{
			"model":           m,
			"id":              fmt.Sprintf("custom:FlameGate-%d", i),
			"baseUrl":         ensureSuffix(baseURL, "/v1"),
			"apiKey":          apiKey,
			"displayName":     m,
			"maxOutputTokens": 131072,
			"provider":        "openai",
		}
	}
	if len(customModels) == 0 {
		customModels = []map[string]any{{
			"model":           "gpt-4o",
			"id":              "custom:FlameGate-0",
			"baseUrl":         ensureSuffix(baseURL, "/v1"),
			"apiKey":          apiKey,
			"displayName":     "gpt-4o (FlameGate)",
			"maxOutputTokens": 131072,
			"provider":        "openai",
		}}
	}
	cfg["customModels"] = customModels
	if len(models) > 0 {
		cfg["activeModel"] = "custom:FlameGate-0"
	}
	return writeJSON(path, cfg)
}

func (t *DroidTool) Remove(homeDir string) error {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return nil
	}
	var cfg map[string]any
	if err := readJSON(path, &cfg); err != nil {
		return err
	}
	delete(cfg, "customModels")
	delete(cfg, "activeModel")
	if len(cfg) == 0 {
		return os.Remove(path)
	}
	return writeJSON(path, cfg)
}
