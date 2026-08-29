package clitools

import (
	"os"
)

// ClineTool auto-configures Cline/Roo (~/.cline/data/globalState.json + secrets.json).
type ClineTool struct{}

func (t *ClineTool) ID() string   { return "cline" }
func (t *ClineTool) Name() string { return "Cline / Roo" }

func (t *ClineTool) globalStatePath(homeDir string) string {
	return expandHome(homeDir, "~/.cline/data/globalState.json")
}
func (t *ClineTool) secretsPath(homeDir string) string {
	return expandHome(homeDir, "~/.cline/data/secrets.json")
}

func (t *ClineTool) DetectStatus(homeDir string) (bool, bool, string, error) {
	path := t.globalStatePath(homeDir)
	if !fileExists(path) {
		return false, false, path, nil
	}
	var cfg map[string]any
	if err := readJSON(path, &cfg); err != nil {
		return true, false, path, err
	}
	v, _ := cfg["openAiBaseUrl"].(string)
	configured := v != "" && containsLocalhost(v)
	return true, configured, path, nil
}

func (t *ClineTool) Configure(homeDir, baseURL, apiKey string, models []string) error {
	gsPath := t.globalStatePath(homeDir)
	secPath := t.secretsPath(homeDir)

	cfg := make(map[string]any)
	_ = readJSON(gsPath, &cfg)

	cfg["actModeApiProvider"] = "openai"
	cfg["planModeApiProvider"] = "openai"
	cfg["openAiBaseUrl"] = stripSuffix(baseURL, "/v1")
	if len(models) > 0 {
		cfg["openAiModelId"] = models[0]
		cfg["planModeOpenAiModelId"] = models[0]
	}

	if err := writeJSON(gsPath, cfg); err != nil {
		return err
	}

	sec := make(map[string]any)
	_ = readJSON(secPath, &sec)
	sec["openAiApiKey"] = apiKey
	return writeJSON(secPath, sec)
}

func (t *ClineTool) Remove(homeDir string) error {
	gsPath := t.globalStatePath(homeDir)
	secPath := t.secretsPath(homeDir)

	if fileExists(gsPath) {
		var cfg map[string]any
		if err := readJSON(gsPath, &cfg); err == nil {
			for _, k := range []string{
				"actModeApiProvider", "planModeApiProvider",
				"openAiBaseUrl", "openAiModelId", "planModeOpenAiModelId",
			} {
				delete(cfg, k)
			}
			if len(cfg) == 0 {
				os.Remove(gsPath)
			} else if err := writeJSON(gsPath, cfg); err != nil {
				return err
			}
		}
	}

	if fileExists(secPath) {
		var sec map[string]any
		if err := readJSON(secPath, &sec); err == nil {
			delete(sec, "openAiApiKey")
			if len(sec) == 0 {
				os.Remove(secPath)
			} else if err := writeJSON(secPath, sec); err != nil {
				return err
			}
		}
	}
	return nil
}
