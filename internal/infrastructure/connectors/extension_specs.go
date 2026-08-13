package connectors

import (
	"sync"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

var (
	extMu    sync.RWMutex
	extSpecs = map[string]ProviderSpec{}
)

// RegisterExtensionSpec projects an installed WASM extension into Catalog/SpecByID.
func RegisterExtensionSpec(spec ProviderSpec) {
	if spec.ID == "" {
		return
	}
	if spec.DisplayName == "" {
		spec.DisplayName = spec.ID
	}
	if spec.Alias == "" {
		spec.Alias = spec.ID
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
