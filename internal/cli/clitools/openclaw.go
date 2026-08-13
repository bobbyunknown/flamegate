package clitools

import (
	"os"
)

// OpenClawTool auto-configures OpenClaw (~/.openclaw/openclaw.json).
type OpenClawTool struct{}

func (t *OpenClawTool) ID() string   { return "openclaw" }
func (t *OpenClawTool) Name() string { return "OpenClaw" }

func (t *OpenClawTool) configPath(homeDir string) string {
	return expandHome(homeDir, "~/.openclaw/openclaw.json")
}

func (t *OpenClawTool) DetectStatus(homeDir string) (bool, bool, string, error) {
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

func (t *OpenClawTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	path := t.configPath(homeDir)
	cfg := make(map[string]any)
	_ = readJSON(path, &cfg)

	provider := map[string]any{
		"baseUrl": ensureSuffix(baseURL, "/v1"),
		"apiKey":  apiKey,
		"api":     "openai-completions",
	}
	if len(models) > 0 {
		provider["models"] = models
	}

	modelsMap, _ := cfg["models"].(map[string]any)
	if modelsMap == nil {
		modelsMap = make(map[string]any)
	}
	modelsMap["providers"] = map[string]any{"flamegate": provider}
	cfg["models"] = modelsMap

	if len(models) > 0 {
		agents, _ := cfg["agents"].(map[string]any)
		if agents == nil {
			agents = make(map[string]any)
		}
		defaults, _ := agents["defaults"].(map[string]any)
		if defaults == nil {
			defaults = make(map[string]any)
		}
		defaults["model"] = map[string]any{"primary": "flamegate/" + models[0]}
		agents["defaults"] = defaults
		cfg["agents"] = agents
	}

	return writeJSON(path, cfg)
}

func (t *OpenClawTool) Remove(homeDir string) error {
	path := t.configPath(homeDir)
	if !fileExists(path) {
		return nil
	}
	var cfg map[string]any
	if err := readJSON(path, &cfg); err != nil {
		return err
	}
	if models, ok := cfg["models"].(map[string]any); ok {
		if providers, ok := models["providers"].(map[string]any); ok {
			delete(providers, "flamegate")
		}
	}
	if agents, ok := cfg["agents"].(map[string]any); ok {
		if defaults, ok := agents["defaults"].(map[string]any); ok {
			if model, ok := defaults["model"].(map[string]any); ok {
				if primary, ok := model["primary"].(string); ok && containsFlameGate(primary) {
					delete(defaults, "model")
				}
			}
		}
	}
	if len(cfg) == 0 {
		return os.Remove(path)
	}
	return writeJSON(path, cfg)
}
