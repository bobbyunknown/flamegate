package catalog

// RawCatalog is the root structure of models.dev/api.json keyed by provider ID.
type RawCatalog map[string]RawProvider

// RawProvider represents a model provider entry in models.dev.
type RawProvider struct {
	ID     string              `json:"id"`
	Name   string              `json:"name"`
	Models map[string]RawModel `json:"models"`
}

// RawModel represents a single model definition in models.dev.
type RawModel struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	ToolCall    bool           `json:"tool_call"` // key in models.dev is "tool_call"
	Reasoning   bool           `json:"reasoning"`
	Modalities  *RawModalities `json:"modalities,omitempty"`
	Cost        *RawCost       `json:"cost,omitempty"`
	Limit       *RawLimit      `json:"limit,omitempty"`
}

// RawModalities represents input and output modality capabilities.
type RawModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// RawCost represents per-million token pricing in USD.
type RawCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
	Reasoning  float64 `json:"reasoning"`
}

// RawLimit represents token limits for context and output.
type RawLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}
