package clitools

import (
	"fmt"
	"os"
	"strings"
)

// CopilotTool auto-configures VS Code Copilot (chatLanguageModels.json).
type CopilotTool struct{}

func (t *CopilotTool) ID() string   { return "copilot" }
func (t *CopilotTool) Name() string { return "GitHub Copilot (VS Code)" }

func (t *CopilotTool) configPath(homeDir string) string {
	return platformConfigPath(homeDir, "chatLanguageModels.json")
}

func (t *CopilotTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	var entries []map[string]any
	if err := readJSON(path, &entries); err != nil {
		// Might be an object, not array — still installed.
		return true, false, path, nil //nolint:nilerr // file format unknown, treat as installed
	}
	for _, e := range entries {
		if name, _ := e["name"].(string); name == "FlameGate" {
			return true, true, path, nil
		}
	}
	return true, false, path, nil
}

func (t *CopilotTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	path := t.configPath(homeDir)

	var entries []map[string]any
	_ = readJSON(path, &entries)
	if entries == nil {
		entries = []map[string]any{}
	}

	// Remove existing FlameGate entry.
	filtered := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if name, _ := e["name"].(string); name != "FlameGate" {
			filtered = append(filtered, e)
		}
	}

	// Build models array.
	modelEntries := make([]map[string]any, len(models))
	for i, m := range models {
		modelEntries[i] = map[string]any{
			"id":              m,
			"name":            m,
			"url":             fmt.Sprintf("%s/v1/chat/completions#models.ai.azure.com", baseURL),
			"toolCalling":     true,
			"maxInputTokens":  128000,
			"maxOutputTokens": 16384,
		}
	}
	if len(modelEntries) == 0 {
		modelEntries = []map[string]any{{
			"id":              "gpt-4o",
			"name":            "gpt-4o",
			"url":             fmt.Sprintf("%s/v1/chat/completions#models.ai.azure.com", baseURL),
			"toolCalling":     true,
			"maxInputTokens":  128000,
			"maxOutputTokens": 16384,
		}}
	}

	entry := map[string]any{
		"name":   "FlameGate",
		"vendor": "azure",
		"apiKey": apiKey,
		"models": modelEntries,
	}
	filtered = append(filtered, entry)

	return writeJSON(path, filtered)
}

func (t *CopilotTool) Remove(homeDir string) error {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return nil
	}
	var entries []map[string]any
	if err := readJSON(path, &entries); err != nil {
		return err
	}
	filtered := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if name, _ := e["name"].(string); name != "FlameGate" {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		return os.Remove(path)
	}
	return writeJSON(path, filtered)
}

// containsFlameGate reports whether a JSON string contains a FlameGate marker.
func containsFlameGate(s string) bool {
	return strings.Contains(s, "FlameGate") || strings.Contains(s, "flamegate")
}
