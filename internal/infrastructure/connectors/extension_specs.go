package connectors

import (
	"sync"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

var (
	extMu    sync.RWMutex
	extSpecs = map[string]ProviderSpec{}
)

var defaultExtensionAliases = map[string]struct {
	Alias   string
	Aliases []string
}{
	"antigravity": {Alias: "agy", Aliases: []string{"agy", "antigravity"}},
	"cline":       {Alias: "cl", Aliases: []string{"cl", "cline"}},
	"xiaomi-mimo": {Alias: "mimo", Aliases: []string{"mimo", "xm", "xiaomi-mimo"}},
}

// RegisterExtensionSpec projects an installed WASM extension into Catalog/SpecByID.
func RegisterExtensionSpec(spec ProviderSpec) {
	if spec.ID == "" {
		return
	}
	if spec.DisplayName == "" {
		spec.DisplayName = spec.ID
	}
	if spec.Alias == "" {
		if def, ok := defaultExtensionAliases[spec.ID]; ok {
			spec.Alias = def.Alias
			if len(spec.Aliases) == 0 {
				spec.Aliases = def.Aliases
			}
		} else {
			spec.Alias = spec.ID
		}
	}
	if len(spec.Aliases) == 0 {
		spec.Aliases = []string{spec.Alias, spec.ID}
	}
	if spec.Dialect == "" {
		spec.Dialect = core.DialectOpenAI
	}
	if spec.AuthKind == "" {
		spec.AuthKind = "api_key"
	}
	if len(spec.ServiceKinds) == 0 {
		spec.ServiceKinds = llm()
	}
	spec.Custom = false
	if spec.Notice == "" {
		spec.Notice = "WASM extension"
	}
	extMu.Lock()
	defer extMu.Unlock()
	extSpecs[spec.ID] = spec
}

// UnregisterExtensionSpec removes an extension projection from the catalog.
func UnregisterExtensionSpec(slug string) {
	extMu.Lock()
	defer extMu.Unlock()
	delete(extSpecs, slug)
}

func extensionSpecs() []ProviderSpec {
	extMu.RLock()
	defer extMu.RUnlock()
	out := make([]ProviderSpec, 0, len(extSpecs))
	for _, p := range extSpecs {
		out = append(out, p)
	}
	return out
}
