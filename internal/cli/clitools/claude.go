package clitools

import (
	"os"
	"strings"
)

// ClaudeTool auto-configures Claude Code (~/.claude/settings.json).
type ClaudeTool struct{}

func (t *ClaudeTool) ID() string   { return "claude" }
func (t *ClaudeTool) Name() string { return "Claude Code" }

func (t *ClaudeTool) configPath(homeDir string) string {
	return expandHome(homeDir, "~/.claude/settings.json")
}

func (t *ClaudeTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	var cfg map[string]any
	if err := readJSON(path, &cfg); err != nil {
		return true, false, path, err
	}
	return true, t.hasFlameGate(cfg), path, nil
}

func (t *ClaudeTool) hasFlameGate(cfg map[string]any) bool {
	env, _ := cfg["env"].(map[string]any)
	if env == nil {
		return false
	}
	v, _ := env["ANTHROPIC_BASE_URL"].(string)
	return v != ""
}

func (t *ClaudeTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	path := t.configPath(homeDir)
	cfg := make(map[string]any)
	_ = readJSON(path, &cfg)

	env, _ := cfg["env"].(map[string]any)
	if env == nil {
		env = make(map[string]any)
	}
	// The Anthropic SDK appends /v1/messages to ANTHROPIC_BASE_URL, so we must
	// NOT include /v1 in the base URL to avoid double /v1/v1/messages.
	env["ANTHROPIC_BASE_URL"] = stripSuffix(baseURL, "/v1")
	env["ANTHROPIC_AUTH_TOKEN"] = apiKey
	if len(models) >= 1 {
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = models[0]
	}
	if len(models) >= 2 {
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = models[1]
	}
	if len(models) >= 3 {
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = models[2]
	}
	cfg["env"] = env
	cfg["hasCompletedOnboarding"] = true

	return writeJSON(path, cfg)
}

func (t *ClaudeTool) Remove(homeDir string) error {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return nil
	}
	var cfg map[string]any
	if err := readJSON(path, &cfg); err != nil {
		return err
	}
	env, _ := cfg["env"].(map[string]any)
	if env != nil {
		for _, k := range []string{
			"ANTHROPIC_BASE_URL",
			"ANTHROPIC_AUTH_TOKEN",
			"ANTHROPIC_DEFAULT_OPUS_MODEL",
			"ANTHROPIC_DEFAULT_SONNET_MODEL",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL",
		} {
			delete(env, k)
		}
		if len(env) == 0 {
			delete(cfg, "env")
		}
	}
	delete(cfg, "hasCompletedOnboarding")

	// If config is now empty, remove the file.
	if len(cfg) == 0 {
		return os.Remove(path)
	}
	return writeJSON(path, cfg)
}

// containsLocalhost reports whether s contains a localhost/127.0.0.1 marker.
func containsLocalhost(s string) bool {
	return strings.Contains(s, "localhost") ||
		strings.Contains(s, "127.0.0.1") ||
		strings.Contains(s, "0.0.0.0")
}
