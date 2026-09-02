package connectors

import (
	"context"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// ModelSpec describes a single model offered by a provider, tagged with the
// service kind it serves. It backs the per-kind discovery endpoints
// (GET /v1/models/<kind>) and model info lookups (GET /v1/models/info).
type ModelSpec struct {
	// ID is the model id the client passes (provider-local, no alias prefix).
	ID string `json:"id"`
	// Name is a human-friendly display name.
	Name string `json:"name"`
	// Kind is the service kind this model serves (defaults to LLM).
	Kind core.ServiceKind `json:"kind"`
	// Dimensions is the embedding vector width (embedding models only).
	Dimensions int `json:"dimensions,omitempty"`
}

// m tags an LLM model.
func m(id, name string) ModelSpec { return ModelSpec{ID: id, Name: name, Kind: core.ServiceLLM} }

// providerModels maps a provider id to the curated set of models it offers.
// In an extension-first architecture, extensions discover models via WASM list_models and users register custom models.
var providerModels = map[string][]ModelSpec{}

// ModelsForProvider returns the curated model list for a provider id, merging
// the static catalog with any user-registered custom models. Custom models
// override static entries with the same id.
func ModelsForProvider(providerID string) []ModelSpec {
	static := providerModels[providerID]
	custom := dynamicModelsFor(providerID)
	if len(custom) == 0 {
		return static
	}
	if len(static) == 0 {
		return custom
	}
	merged := make([]ModelSpec, 0, len(static)+len(custom))
	customByID := make(map[string]bool, len(custom))
	for _, m := range custom {
		customByID[m.ID] = true
	}
	for _, m := range static {
		if customByID[m.ID] {
			continue // custom entry takes precedence
		}
		merged = append(merged, m)
	}
	return append(merged, custom...)
}

// ModelsByKind returns all (providerID, model) pairs across the catalog that
// serve the given service kind, excluding hidden providers.
type ProviderModel struct {
	Provider string
	Model    ModelSpec
}

// ModelsByKind collects every model of the given kind across all non-hidden
// providers in the catalog.
func ModelsByKind(kind core.ServiceKind) []ProviderModel {
	var out []ProviderModel
	for _, spec := range Catalog() {
		if spec.Hidden {
			continue
		}
		if !core.HasServiceKind(spec.ServiceKinds, kind) {
			continue
		}
		for _, mdl := range ModelsForProvider(spec.ID) {
			if mdl.Kind == kind {
				out = append(out, ProviderModel{Provider: spec.ID, Model: mdl})
			}
		}
	}
	return out
}

// FindModel locates a model by provider id and model id.
func FindModel(providerID, modelID string) (ModelSpec, bool) {
	for _, mdl := range ModelsForProvider(providerID) {
		if mdl.ID == modelID {
			return mdl, true
		}
	}
	return ModelSpec{}, false
}

// LiveModelSource is implemented by connectors that can fetch their model
// catalog from the upstream API at runtime (e.g. Kiro's ListAvailableModels).
// The gateway uses this to supplement the static providerModels catalog with
// live data when an account is connected.
type LiveModelSource interface {
	// ListModels fetches the live model catalog from the upstream. The returned
	// models should already include any synthetic variants (e.g. Kiro's
	// -thinking/-agentic expansions). The creds carry the access token needed
	// to authenticate with the upstream.
	ListModels(ctx context.Context, creds core.Credentials) ([]ModelSpec, error)
}

// liveModelSources is the registry of providers that support live model
// discovery. Populated at init time.
var liveModelSources = map[string]LiveModelSource{}

// RegisterLiveModelSource registers a live model source for a provider.
func RegisterLiveModelSource(provider string, src LiveModelSource) {
	liveModelSources[provider] = src
}

// GetLiveModelSource returns the live model source for a provider, or nil.
func GetLiveModelSource(provider string) LiveModelSource {
	if src, ok := liveModelSources[provider]; ok {
		return src
	}
	// Dynamic (user-defined) OpenAI-compatible providers are not in the static
	// registry, so build a discovery source on demand. This lets a custom
	// provider's /models endpoint populate the catalog just like a built-in one.
	if p, ok := DynamicProviderByID(provider); ok && p.Dialect == core.DialectOpenAI {
		return &OpenAICompatibleModelSource{provider: p.ID, defaultBase: p.BaseURL}
	}
	return nil
}

// QuotaEntry is one upstream quota bucket (e.g. AGENTIC_REQUEST usage).
type QuotaEntry struct {
	ResourceType string `json:"resource_type"`
	Used         int    `json:"used"`
	Limit        int    `json:"limit"`
	Remaining    int    `json:"remaining"`
	ResetAt      string `json:"reset_at,omitempty"`
	PlanName     string `json:"plan_name,omitempty"`
}

// QuotaResult holds the upstream quota info for an account.
type QuotaResult struct {
	PlanName string       `json:"plan_name,omitempty"`
	Quotas   []QuotaEntry `json:"quotas"`
	Message  string       `json:"message,omitempty"`
}

// QuotaSource is implemented by connectors that can fetch upstream quota/usage
// info (e.g. Kiro's getUsageLimits).
type QuotaSource interface {
	FetchQuota(ctx context.Context, creds core.Credentials) (*QuotaResult, error)
}

var quotaSources = map[string]QuotaSource{}

// RegisterQuotaSource registers a quota source for a provider.
func RegisterQuotaSource(provider string, src QuotaSource) {
	quotaSources[provider] = src
}

// GetQuotaSource returns the quota source for a provider, or nil.
func GetQuotaSource(provider string) QuotaSource {
	return quotaSources[provider]
}
