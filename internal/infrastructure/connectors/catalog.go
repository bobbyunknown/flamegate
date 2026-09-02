package connectors

import (
	"strings"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// RegionOption describes one selectable region for a provider that has
// region-based endpoints (e.g. Xiaomi Token Plan SGP/CN/AMS).
type RegionOption struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	BaseURL string `json:"base_url"`
}

// ProviderSpec describes a built-in provider: its id, the wire dialect it
// speaks, its default endpoint, the service kinds it serves, and the metadata
// the dashboard renders (display name, alias, brand color, auth modes, etc.).
type ProviderSpec struct {
	ID          string
	DisplayName string
	// Alias is the short code accepted in model strings (e.g. "agy" -> antigravity).
	Alias string
	// Aliases lists all alternative accepted short codes.
	Aliases []string
	Dialect core.Dialect
	BaseURL string
	// AuthKind is the default authentication mechanism (api_key, oauth, none).
	AuthKind string
	// AuthModes lists every supported auth mechanism (a provider may offer both
	// OAuth and API key). Defaults to [AuthKind] when empty.
	AuthModes []string
	// ServiceKinds enumerates the capabilities this provider serves. Empty means
	// LLM-only (the conservative default).
	ServiceKinds []core.ServiceKind
	// Color is the brand color used by the dashboard (hex).
	Color string
	// Website is the provider's marketing/console URL.
	Website string
	// APIKeyURL is where a user mints an API key for this provider.
	APIKeyURL string
	// Deprecated marks providers that carry usage risk (unofficial OAuth, etc.).
	Deprecated bool
	// Hidden hides the provider from the default dashboard listing.
	Hidden bool
	// Pinned providers appear at the top of the dashboard listing.
	Pinned bool
	// SkipValidation skips upstream credential probing during account creation.
	// Used for providers behind WAF/CDN that block server-side requests.
	SkipValidation bool
	// Notice is a short human-readable note shown in the dashboard.
	Notice string
	// Pricing (USD per million tokens) used for cost estimation. Zero means
	// free or unknown.
	InputPerM  float64
	OutputPerM float64
	// Regions lists selectable endpoint regions for providers with region-based
	// URLs (e.g. Xiaomi Token Plan). When set, the dashboard shows a region
	// dropdown instead of a free-text base URL field.
	Regions []RegionOption `json:"regions,omitempty"`
	// DefaultRegion is the pre-selected region id when Regions is non-empty.
	DefaultRegion string `json:"default_region,omitempty"`
	// Custom marks user-defined dynamic provider instances (not part of the
	// built-in static catalog). These are editable and deletable.
	Custom bool `json:"custom,omitempty"`
}

// llm is shorthand for the default LLM-only service kind slice.
func llm(extra ...core.ServiceKind) []core.ServiceKind {
	return append([]core.ServiceKind{core.ServiceLLM}, extra...)
}

// Catalog returns static native providers, runtime dynamic customs, and
// installed WASM extension projections.
func Catalog() []ProviderSpec {
	return append(append(nativeProviders(), dynamicSpecs()...), extensionSpecs()...)
}

// IsNativeSlug reports whether slug is one of the static custom template providers.
// It must not consult Catalog() — dynamic/extension rows are not native.
func IsNativeSlug(slug string) bool {
	switch strings.ToLower(slug) {
	case "custom-openai", "custom-anthropic", "custom-gemini":
		return true
	default:
		return false
	}
}

// nativeProviders returns internal template specs used for dynamic custom providers.
func nativeProviders() []ProviderSpec {
	return []ProviderSpec{
		{
			ID: "custom-openai", DisplayName: "Custom (OpenAI-compatible)", Alias: "custom-openai",
			Dialect: core.DialectOpenAI, BaseURL: "", AuthKind: "api_key", ServiceKinds: llm(), Pinned: true,
		},
		{
			ID: "custom-anthropic", DisplayName: "Custom (Anthropic-compatible)", Alias: "custom-anthropic",
			Dialect: core.DialectAnthropic, BaseURL: "", AuthKind: "api_key", ServiceKinds: llm(), Pinned: true,
		},
		{
			ID: "custom-gemini", DisplayName: "Custom (Gemini-compatible)", Alias: "custom-gemini",
			Dialect: core.DialectGemini, BaseURL: "", AuthKind: "api_key", ServiceKinds: llm(), Pinned: true,
		},
	}
}

// SpecByID returns the catalog spec for a provider id, or false if unknown.
func SpecByID(id string) (ProviderSpec, bool) {
	for _, p := range Catalog() {
		if p.ID == id {
			return p, true
		}
	}
	return ProviderSpec{}, false
}

// SpecByAlias resolves a provider by its short alias or id.
func SpecByAlias(aliasOrID string) (ProviderSpec, bool) {
	aliasOrID = strings.ToLower(strings.TrimSpace(aliasOrID))
	if aliasOrID == "" {
		return ProviderSpec{}, false
	}
	for _, p := range Catalog() {
		if strings.EqualFold(p.ID, aliasOrID) || strings.EqualFold(p.Alias, aliasOrID) {
			return p, true
		}
		for _, a := range p.Aliases {
			if strings.EqualFold(a, aliasOrID) {
				return p, true
			}
		}
	}
	return ProviderSpec{}, false
}

// SpecsByKind returns the catalog specs that serve a given service kind,
// excluding hidden providers.
func SpecsByKind(kind core.ServiceKind) []ProviderSpec {
	var out []ProviderSpec
	for _, p := range Catalog() {
		if p.Hidden {
			continue
		}
		for _, k := range p.ServiceKinds {
			if k == kind {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

// ResolveRegionBaseURL returns the base URL for the given region of a provider.
// If the region is empty or unknown, the default region's URL is returned.
// Returns "" if the provider has no regions.
func ResolveRegionBaseURL(providerID, regionID string) string {
	spec, ok := SpecByID(providerID)
	if !ok || len(spec.Regions) == 0 {
		return ""
	}
	if regionID == "" {
		regionID = spec.DefaultRegion
	}
	for _, r := range spec.Regions {
		if r.ID == regionID {
			return r.BaseURL
		}
	}
	if spec.DefaultRegion != "" {
		for _, r := range spec.Regions {
			if r.ID == spec.DefaultRegion {
				return r.BaseURL
			}
		}
	}
	return spec.Regions[0].BaseURL
}

// AuthModesOf returns the auth modes for a spec, defaulting to [AuthKind].
func (p ProviderSpec) AuthModesOf() []string {
	if len(p.AuthModes) > 0 {
		return p.AuthModes
	}
	if p.AuthKind != "" {
		return []string{p.AuthKind}
	}
	return nil
}
