package catalog

import "strings"

// ModelLimits contains context window and maximum output tokens.
type ModelLimits struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

// ModelCost contains per-million token pricing in USD.
type ModelCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Reasoning  float64 `json:"reasoning"`
}

// ModelModalities contains supported input and output modalities.
type ModelModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// ModelSpec represents a normalized LLM model specification.
type ModelSpec struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Provider    string          `json:"provider"`
	Description string          `json:"description,omitempty"`
	Limits      ModelLimits     `json:"limits"`
	Cost        ModelCost       `json:"cost"`
	Modalities  ModelModalities `json:"modalities"`
	Reasoning   bool            `json:"reasoning"`
	ToolCall    bool            `json:"tool_call"`
}

// HasTools reports whether the model supports tool/function calling.
func (m *ModelSpec) HasTools() bool {
	return m.ToolCall
}

// HasVision reports whether the model accepts image or vision inputs.
func (m *ModelSpec) HasVision() bool {
	for _, in := range m.Modalities.Input {
		if strings.EqualFold(in, "image") || strings.EqualFold(in, "vision") {
			return true
		}
	}
	return false
}

// HasPDF reports whether the model accepts PDF inputs.
func (m *ModelSpec) HasPDF() bool {
	for _, in := range m.Modalities.Input {
		if strings.EqualFold(in, "pdf") {
			return true
		}
	}
	return false
}

// HasAudioInput reports whether the model accepts audio inputs.
func (m *ModelSpec) HasAudioInput() bool {
	for _, in := range m.Modalities.Input {
		if strings.EqualFold(in, "audio") {
			return true
		}
	}
	return false
}

// HasVideoInput reports whether the model accepts video inputs.
func (m *ModelSpec) HasVideoInput() bool {
	for _, in := range m.Modalities.Input {
		if strings.EqualFold(in, "video") {
			return true
		}
	}
	return false
}

// ExtractBaseModelSlug strips vendor prefix and version tags, returning a lowercase base slug.
func ExtractBaseModelSlug(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if i := strings.LastIndex(modelID, "/"); i >= 0 {
		modelID = modelID[i+1:]
	}
	if i := strings.Index(modelID, ":"); i >= 0 {
		modelID = modelID[:i]
	}
	modelID = strings.ToLower(strings.TrimSpace(modelID))
	suffixes := []string{"-high", "-low", "-medium", "-thinking", "-search"}
	for _, s := range suffixes {
		if strings.HasSuffix(modelID, s) {
			modelID = strings.TrimSuffix(modelID, s)
			break
		}
	}
	return modelID
}
