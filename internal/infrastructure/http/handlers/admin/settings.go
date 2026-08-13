package admin

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/dispatch"
	"github.com/bobbyunknown/flamegate/internal/shared/caveman"
	"github.com/bobbyunknown/flamegate/internal/shared/ponytail"
)

// endpointSettingsKey is the settings-store key under which token-saving
// preferences are persisted as a JSON blob (RTK token saver + caveman output
// compression + terse mode).
const endpointSettingsKey = "endpoint_settings"

// EndpointSettings holds the dashboard-configurable token-saving preferences.
type EndpointSettings struct {
	// RTKEnabled toggles input-side tool-output compression (the slimmer).
	RTKEnabled     bool   `json:"rtk_enabled"`
	RTKFilterLevel string `json:"rtk_filter_level"` // "none", "minimal", "aggressive"

	// CavemanEnabled toggles output-side caveman compression; CavemanLevel is
	// one of "lite", "full", "ultra".
	CavemanEnabled bool   `json:"caveman_enabled"`
	CavemanLevel   string `json:"caveman_level"`

	// TerseEnabled toggles terse mode, which serializes messages and tools into
	// the compact TERSE format for token-efficient context.
	TerseEnabled bool   `json:"terse_enabled"`
	TerseLevel   string `json:"terse_level"`

	// Headroom (input-side proxy compression).
	HeadroomEnabled              bool   `json:"headroom_enabled"`
	HeadroomURL                  string `json:"headroom_url"`
	HeadroomCompressUserMessages bool   `json:"headroom_compress_user_messages"`
	HeadroomTimeoutMs            int    `json:"headroom_timeout_ms"`

	// Ponytail (output-side system-prompt injection).
	PonytailEnabled bool   `json:"ponytail_enabled"`
	PonytailLevel   string `json:"ponytail_level"` // "lite" | "full" | "ultra"

	// Routing strategy fields.
	RoutingStrategy  string `json:"routing_strategy"`   // "fill-first" | "round-robin" | "smart-round-robin"
	StickyLimit      int    `json:"sticky_limit"`       // calls per account before switching
	ComboStrategy    string `json:"combo_strategy"`     // "fallback" | "round-robin"
	ComboStickyLimit int    `json:"combo_sticky_limit"` // calls per combo model before switching

	// Outbound proxy settings.
	OutboundProxyEnabled bool   `json:"outbound_proxy_enabled"`
	OutboundProxyURL     string `json:"outbound_proxy_url"`
	OutboundNoProxy      string `json:"outbound_no_proxy"`

	// Observability.
	ObservabilityEnabled *bool `json:"observability_enabled,omitempty"`

	// RateLimitsEnabled toggles per-key RPM/TPM/concurrency enforcement.
	RateLimitsEnabled bool `json:"rate_limits_enabled"`

	// Timeout settings (milliseconds).
	// StreamStallTimeoutMs aborts a stream that produces no data for this long.
	// Increase for reasoning models (Deepseek, GLM) that think before streaming.
	StreamStallTimeoutMs int `json:"stream_stall_timeout_ms"`
	// ResponseHeaderTimeoutMs is the max time waiting for upstream response
	// headers. Accommodates slow providers (ollama on modest hardware).
	ResponseHeaderTimeoutMs int `json:"response_header_timeout_ms"`
	// RequestTimeoutMs bounds non-streaming upstream calls.
	RequestTimeoutMs int `json:"request_timeout_ms"`
}

// defaultEndpointSettings returns the defaults: RTK on, caveman/terse off.
func defaultEndpointSettings() EndpointSettings {
	return EndpointSettings{
		RTKEnabled:                   true,
		RTKFilterLevel:               "none",
		CavemanEnabled:               false,
		CavemanLevel:                 string(caveman.LevelFull),
		TerseEnabled:                 false,
		TerseLevel:                   "medium",
		HeadroomEnabled:              false,
		HeadroomURL:                  "",
		HeadroomCompressUserMessages: false,
		HeadroomTimeoutMs:            3000,
		PonytailEnabled:              false,
		PonytailLevel:                string(ponytail.LevelFull),
		StickyLimit:                  3,
		ComboStrategy:                "fallback",
		ComboStickyLimit:             1,
		OutboundProxyEnabled:         false,
		OutboundProxyURL:             "",
		OutboundNoProxy:              "",
		RateLimitsEnabled:            true,
		// Timeout defaults (ms). Generous bounds to
		// accommodate slow upstream providers and reasoning models.
		// Keep account routing context-sticky by default: repeated turns from
		// the same conversation prefer the same account, while new conversations
		// still spread across accounts.
		RoutingStrategy:         string(dispatch.StrategySmartRoundRobin),
		StreamStallTimeoutMs:    120000, // 2 min
		ResponseHeaderTimeoutMs: 60000,  // 60s
		RequestTimeoutMs:        300000, // 5 min
	}
}

// loadEndpointSettings reads the persisted settings, falling back to defaults
// when unset or unreadable. It never errors: token-saving is best-effort.
func (s *Handler) loadEndpointSettings(ctx context.Context) EndpointSettings {
	def := defaultEndpointSettings()
	if s.settings == nil {
		return def
	}
	raw, err := s.settings.Get(ctx, endpointSettingsKey)
	if err != nil || raw == "" {
		return def
	}
	var es EndpointSettings
	if err := json.Unmarshal([]byte(raw), &es); err != nil {
		return def
	}
	var rawFields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &rawFields)
	if _, ok := rawFields["rate_limits_enabled"]; !ok {
		es.RateLimitsEnabled = def.RateLimitsEnabled
	}
	// Backfill empty levels with defaults.
	if es.CavemanLevel == "" {
		es.CavemanLevel = def.CavemanLevel
	}
	if es.RTKFilterLevel == "" {
		es.RTKFilterLevel = def.RTKFilterLevel
	}
	if es.TerseLevel == "" {
		es.TerseLevel = def.TerseLevel
	}
	if es.RoutingStrategy == "" {
		es.RoutingStrategy = def.RoutingStrategy
	} else if normalized, ok := normalizeAccountRoutingStrategy(es.RoutingStrategy); ok {
		es.RoutingStrategy = normalized
	} else {
		es.RoutingStrategy = def.RoutingStrategy
	}
	if es.StickyLimit == 0 {
		es.StickyLimit = def.StickyLimit
	}
	if es.ComboStrategy == "" {
		es.ComboStrategy = def.ComboStrategy
	} else if normalized, ok := normalizeComboRoutingStrategy(es.ComboStrategy); ok {
		es.ComboStrategy = normalized
	} else {
		es.ComboStrategy = def.ComboStrategy
	}
	if es.ComboStickyLimit == 0 {
		es.ComboStickyLimit = def.ComboStickyLimit
	}
	// Backfill timeout defaults for existing settings that predate this feature.
	if es.StreamStallTimeoutMs == 0 {
		es.StreamStallTimeoutMs = def.StreamStallTimeoutMs
	}
	if es.ResponseHeaderTimeoutMs == 0 {
		es.ResponseHeaderTimeoutMs = def.ResponseHeaderTimeoutMs
	}
	if es.RequestTimeoutMs == 0 {
		es.RequestTimeoutMs = def.RequestTimeoutMs
	}
	// Backfill saver defaults for settings that predate this feature. The
	// remaining Headroom/Ponytail fields use Go zero-values (false/"") that
	// already match their defaults.
	if es.HeadroomTimeoutMs == 0 {
		es.HeadroomTimeoutMs = def.HeadroomTimeoutMs
	}
	if es.PonytailLevel == "" {
		es.PonytailLevel = def.PonytailLevel
	}
	return es
}

// ---- admin endpoints --------------------------------------------------------

// adminGetEndpointSettings returns the current token-saving preferences.

// adminTestHeadroom probes a Headroom proxy to confirm it is running. It accepts
// an optional JSON body {"url": "...", "timeout_ms": N}; when omitted it falls
// back to the saved Headroom settings. This lets the dashboard validate a proxy
// before (or after) saving, without ever leaking credentials: the probe result
// only carries a masked endpoint.

// adminUpdateEndpointSettings persists token-saving preferences. It accepts a
// partial body and merges it over the current settings, validating enum values.

func normalizeAccountRoutingStrategy(raw string) (string, bool) {
	switch normalizeStrategyToken(raw) {
	case "", "fill-first", "fill_first", "priority", "fallback":
		return "fill-first", true
	case "round-robin", "round_robin", "roundrobin":
		return string(dispatch.StrategyRoundRobin), true
	case "smart-round-robin", "smart_round_robin", "smartroundrobin", "smart":
		return string(dispatch.StrategySmartRoundRobin), true
	default:
		return "", false
	}
}

func normalizeComboRoutingStrategy(raw string) (string, bool) {
	switch normalizeStrategyToken(raw) {
	case "", "fallback", "priority", "fill-first", "fill_first":
		return string(dispatch.StrategyFallback), true
	case "round-robin", "round_robin", "roundrobin":
		return string(dispatch.StrategyRoundRobin), true
	default:
		return "", false
	}
}

func normalizeStrategyToken(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// ---- provider routing settings --------------------------------------------

const providerRoutingPrefix = "provider_routing_"

// ProviderRoutingSettings optionally overrides account routing for one
// provider. Inherit uses the global EndpointSettings account strategy.
type ProviderRoutingSettings struct {
	RoutingStrategy    string `json:"routing_strategy"` // inherit | fill-first | round-robin | smart-round-robin
	StickyLimit        int    `json:"sticky_limit"`
	AffinityTTLMinutes int    `json:"affinity_ttl_minutes"`
}

func defaultProviderRoutingSettings() ProviderRoutingSettings {
	return ProviderRoutingSettings{
		RoutingStrategy:    "inherit",
		StickyLimit:        3,
		AffinityTTLMinutes: int(dispatch.DefaultAffinityTTL / time.Minute),
	}
}

func (s *Handler) loadProviderRoutingSettings(ctx context.Context, provider string) ProviderRoutingSettings {
	def := defaultProviderRoutingSettings()
	if s.settings == nil || provider == "" {
		return def
	}
	raw, err := s.settings.Get(ctx, providerRoutingPrefix+provider)
	if err != nil || raw == "" {
		return def
	}
	var ps ProviderRoutingSettings
	if err := json.Unmarshal([]byte(raw), &ps); err != nil {
		return def
	}
	if normalized, ok := normalizeProviderRoutingStrategy(ps.RoutingStrategy); ok {
		ps.RoutingStrategy = normalized
	} else {
		ps.RoutingStrategy = def.RoutingStrategy
	}
	if ps.StickyLimit <= 0 {
		ps.StickyLimit = def.StickyLimit
	}
	if ps.AffinityTTLMinutes <= 0 {
		ps.AffinityTTLMinutes = def.AffinityTTLMinutes
	}
	return ps
}

func normalizeProviderRoutingStrategy(raw string) (string, bool) {
	switch normalizeStrategyToken(raw) {
	case "", "inherit":
		return "inherit", true
	case "fill-first", "fill_first", "priority", "fallback":
		return "fill-first", true
	case "round-robin", "round_robin", "roundrobin":
		return string(dispatch.StrategyRoundRobin), true
	case "smart-round-robin", "smart_round_robin", "smartroundrobin", "smart":
		return string(dispatch.StrategySmartRoundRobin), true
	default:
		return "", false
	}
}

// ---- access settings --------------------------------------------------------

// accessSettingsKey is the settings-store key for the Endpoints page access
// options (local access, secure tunnel, Tailscale).
const accessSettingsKey = "access_settings"

// AccessSettings holds the endpoint reachability toggles. These are dashboard
// preferences; the actual tunnels are provisioned out-of-band, so the toggles
// record intent and surface the primary endpoint URL.
type AccessSettings struct {
	LocalEnabled  bool   `json:"local_enabled"`
	TunnelEnabled bool   `json:"tunnel_enabled"`
	Tailscale     bool   `json:"tailscale_enabled"`
	TunnelURL     string `json:"tunnel_url,omitempty"`
	TailscaleURL  string `json:"tailscale_url,omitempty"`
}

func defaultAccessSettings() AccessSettings {
	return AccessSettings{LocalEnabled: true, TunnelEnabled: false, Tailscale: false}
}

func (s *Handler) loadAccessSettings(ctx context.Context) AccessSettings {
	def := defaultAccessSettings()
	if s.settings == nil {
		return def
	}
	raw, err := s.settings.Get(ctx, accessSettingsKey)
	if err != nil || raw == "" {
		return def
	}
	var as AccessSettings
	if err := json.Unmarshal([]byte(raw), &as); err != nil {
		return def
	}
	return as
}

// adminGetAccessSettings returns the access options plus the primary endpoint
// URL derived from the request, so the dashboard can render the Endpoints page.

// adminUpdateAccessSettings persists the access option toggles, merging a
// partial body over the current settings.
