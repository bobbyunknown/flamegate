package connectors

import (
	"strings"
	"sync"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// Custom provider id prefixes. Each user-defined provider instance gets a
// unique id under one of these prefixes (e.g. "custom-openai-myvllm") so that
// multiple endpoints of the same base type stay fully isolated: their own base
// URL, accounts, and models. This mirrors the OmniRoute provider-node model.
const (
	CustomOpenAIPrefix    = "custom-openai-"
	CustomAnthropicPrefix = "custom-anthropic-"
	CustomGeminiPrefix    = "custom-gemini-"
)

// DynamicProvider is a runtime-registered provider instance backed by either
// the OpenAI- or Anthropic-compatible dialect.
type DynamicProvider struct {
	ID          string
	DisplayName string
	Alias       string
	Dialect     core.Dialect
	BaseURL     string
}

var (
	dynMu        sync.RWMutex
	dynProviders = map[string]DynamicProvider{}
	dynModels    = map[string][]ModelSpec{} // providerID -> user-defined models
)

// RegisterDynamicProvider adds or replaces a dynamic provider instance.
func RegisterDynamicProvider(p DynamicProvider) {
	dynMu.Lock()
	defer dynMu.Unlock()
	dynProviders[p.ID] = p
}

// UnregisterDynamicProvider removes a dynamic provider instance and its models.
func UnregisterDynamicProvider(id string) {
	dynMu.Lock()
	defer dynMu.Unlock()
	delete(dynProviders, id)
	delete(dynModels, id)
}

// SetDynamicModels replaces the user-defined models for a provider id. The
// provider id may be a dynamic custom provider OR a built-in catalog provider,
// allowing users to register models on any provider.
func SetDynamicModels(providerID string, models []ModelSpec) {
	dynMu.Lock()
	defer dynMu.Unlock()
	if len(models) == 0 {
		delete(dynModels, providerID)
		return
	}
	cp := make([]ModelSpec, len(models))
	copy(cp, models)
	dynModels[providerID] = cp
}

// DynamicProviderByID returns the dynamic provider for an id, or false.
func DynamicProviderByID(id string) (DynamicProvider, bool) {
	dynMu.RLock()
	defer dynMu.RUnlock()
	p, ok := dynProviders[id]
	return p, ok
}

// dynamicSpecs projects the registered dynamic providers into ProviderSpecs so
// they appear in Catalog() and resolve through SpecByID/SpecByAlias.
func dynamicSpecs() []ProviderSpec {
	dynMu.RLock()
	defer dynMu.RUnlock()
	out := make([]ProviderSpec, 0, len(dynProviders))
	for _, p := range dynProviders {
		out = append(out, ProviderSpec{
			ID:           p.ID,
			DisplayName:  p.DisplayName,
			Alias:        p.Alias,
			Dialect:      p.Dialect,
			BaseURL:      p.BaseURL,
			AuthKind:     "api_key",
			ServiceKinds: llm(),
			Custom:       true,
		})
	}
	return out
}

// dynamicModelsFor returns a copy of the user-defined models for a provider id.
func dynamicModelsFor(providerID string) []ModelSpec {
	dynMu.RLock()
	defer dynMu.RUnlock()
	models := dynModels[providerID]
	if len(models) == 0 {
		return nil
	}
	cp := make([]ModelSpec, len(models))
	copy(cp, models)
	return cp
}

// IsCustomProviderID reports whether a provider id belongs to a user-defined
// dynamic provider instance or a generic custom-compatible gateway. These
// providers have unknown upstream capabilities, so the routing layer relaxes
// capability guards for them — unsupported modalities are stripped downstream
// rather than blocking the request.
//
// Matches:
//   - "custom-openai", "custom-anthropic", and "custom-gemini" (the built-in generic gateways)
//   - "custom-openai-*", "custom-anthropic-*", "custom-gemini-*" (user-defined dynamic instances)
func IsCustomProviderID(id string) bool {
	if id == "custom-openai" || id == "custom-anthropic" || id == "custom-gemini" {
		return true
	}
	return strings.HasPrefix(id, CustomOpenAIPrefix) || strings.HasPrefix(id, CustomAnthropicPrefix) || strings.HasPrefix(id, CustomGeminiPrefix)
}

// dynamicConnector builds a connector on demand for a dynamic provider. Custom
// connectors are cheap stateless structs, so building per-lookup is fine.
func dynamicConnector(providerID string) (core.Connector, bool) {
	p, ok := DynamicProviderByID(providerID)
	if !ok {
		return nil, false
	}
	switch p.Dialect {
	case core.DialectAnthropic:
		return NewAnthropic(p.ID, p.BaseURL), true
	case core.DialectOpenAI:
		return NewOpenAICompatible(p.ID, p.BaseURL), true
	case core.DialectGemini:
		return NewGemini(p.ID, p.BaseURL), true
	default:
		return nil, false
	}
}
