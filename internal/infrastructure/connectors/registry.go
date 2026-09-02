package connectors

import (
	"fmt"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// Registry resolves connectors by provider id. Built-in natives are fixed at
// construction; WASM and dynamic providers resolve at lookup time.
type Registry struct {
	byID         map[string]core.Connector
	wasmFallback func(string) (core.Connector, bool) // optional WASM lookup
	forceWasmAll bool                                // when true, even Tier-1 go through WASM
}

// NewRegistry builds a registry from the given connectors.
func NewRegistry(conns ...core.Connector) *Registry {
	m := make(map[string]core.Connector, len(conns))
	for _, c := range conns {
		m[c.ID()] = c
	}
	return &Registry{byID: m}
}

// SetForceWasmAll forces all providers (including Tier-1) through WASM when true.
func (r *Registry) SetForceWasmAll(v bool) { r.forceWasmAll = v }

// SetWASMFallback installs the WASM connector lookup.
// Called after construction (before serving requests). nil disables WASM fallback.
func (r *Registry) SetWASMFallback(fn func(string) (core.Connector, bool)) {
	r.wasmFallback = fn
}

// Get returns the connector for a provider id.
// Resolution order:
//  1. forceWasmAll → WASM if available
//  2. Native connector (byID) if present
//  3. WASM fallback if available
//  4. Dynamic connector
//  5. Error
func (r *Registry) Get(provider string) (core.Connector, error) {
	if r.forceWasmAll && r.wasmFallback != nil {
		if c, ok := r.wasmFallback(provider); ok {
			return c, nil
		}
	}
	if c, ok := r.byID[provider]; ok {
		return c, nil
	}
	if r.wasmFallback != nil {
		if c, ok := r.wasmFallback(provider); ok {
			return c, nil
		}
	}
	if c, ok := dynamicConnector(provider); ok {
		return c, nil
	}
	return nil, fmt.Errorf("connectors: provider %q not available, install extension or use custom-openai/custom-anthropic", provider)
}

// Has reports whether a provider is registered (built-in, WASM, or dynamic).
func (r *Registry) Has(provider string) bool {
	if _, ok := r.byID[provider]; ok {
		return true
	}
	if r.wasmFallback != nil {
		if _, ok := r.wasmFallback(provider); ok {
			return true
		}
	}
	_, ok := DynamicProviderByID(provider)
	return ok
}

// Providers returns the registered provider ids.
func (r *Registry) Providers() []string {
	out := make([]string, 0, len(r.byID))
	for id := range r.byID {
		out = append(out, id)
	}
	return out
}

// DefaultRegistry builds the static custom template connectors.
// Extensions resolve via SetWASMFallback; custom-* dynamic instances via dynamicConnector.
func DefaultRegistry() *Registry {
	conns := []core.Connector{
		NewOpenAICompatible("custom-openai", ""),
		NewAnthropic("custom-anthropic", ""),
		NewGemini("custom-gemini", ""),
	}
	RegisterLiveModelSource("custom-openai", &OpenAICompatibleModelSource{provider: "custom-openai", defaultBase: ""})
	return NewRegistry(conns...)
}

// DrivableDialect reports whether FlameGate has a connector that can drive the
// given upstream dialect today.
func DrivableDialect(d core.Dialect) bool {
	switch d {
	case core.DialectOpenAI, core.DialectAnthropic, core.DialectGemini,
		core.DialectGeminiCLI, core.DialectAntigravity:
		return true
	default:
		return false
	}
}
