package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails/pii"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/openapi"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel/cloudflare"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/update"
	"github.com/bobbyunknown/flamegate/internal/shared/caveman"
	"github.com/bobbyunknown/flamegate/internal/shared/headroom"
	"github.com/bobbyunknown/flamegate/internal/shared/httputil"
	"github.com/bobbyunknown/flamegate/internal/shared/ponytail"
	"github.com/bobbyunknown/flamegate/internal/shared/terse"
)

// --- List Providers ---

type ListProvidersInput struct {
	Kind string `query:"kind" doc:"Optional service kind filter (llm, tts, stt, embeddings, moderation, image-generation, web-search)"`
}

type ListProvidersOutput struct {
	Body struct {
		Providers []map[string]any `json:"providers" doc:"List of available providers"`
	}
}

func (s *Handler) HumaListProviders(ctx context.Context, input *ListProvidersInput) (*ListProvidersOutput, error) {
	kindFilter := core.ServiceKind(input.Kind)

	specs := connectors.Catalog()
	out := make([]map[string]any, 0, len(specs))
	for _, p := range specs {
		if kindFilter != "" && !core.HasServiceKind(p.ServiceKinds, kindFilter) {
			continue
		}
		kinds := p.ServiceKinds
		if len(kinds) == 0 {
			kinds = []core.ServiceKind{core.ServiceLLM}
		}
		entry := map[string]any{
			"id":            p.ID,
			"display_name":  p.DisplayName,
			"alias":         p.Alias,
			"dialect":       p.Dialect,
			"auth_kind":     p.AuthKind,
			"auth_modes":    p.AuthModesOf(),
			"service_kinds": kinds,
			"color":         p.Color,
			"website":       p.Website,
			"api_key_url":   p.APIKeyURL,
			"icon":          "/providers/" + p.ID + ".png",
			"deprecated":    p.Deprecated,
			"hidden":        p.Hidden,
			"pinned":        p.Pinned,
			"notice":        p.Notice,
			"drivable":      connectors.DrivableDialect(p.Dialect) || webProvider(p.ID),
			"input_per_m":   p.InputPerM,
			"output_per_m":  p.OutputPerM,
		}
		if p.Custom {
			entry["custom"] = true
			entry["base_url"] = p.BaseURL
		}
		if len(p.Regions) > 0 {
			regions := make([]map[string]string, 0, len(p.Regions))
			for _, r := range p.Regions {
				regions = append(regions, map[string]string{
					"id":       r.ID,
					"label":    r.Label,
					"base_url": r.BaseURL,
				})
			}
			entry["regions"] = regions
			entry["default_region"] = p.DefaultRegion
		}
		out = append(out, entry)
	}
	return &ListProvidersOutput{Body: struct {
		Providers []map[string]any `json:"providers" doc:"List of available providers"`
	}{Providers: out}}, nil
}

// --- Provider Models ---

type ProviderModelsInput struct {
	// id = providers[].id from GET /api/providers (slug, e.g. openai or xiaomi-mimo). Not a UUID.
	ID      string `path:"id" doc:"Provider id from GET /providers → providers[].id (e.g. openai, xiaomi-mimo)" example:"xiaomi-mimo"`
	Kind    string `query:"kind" doc:"Optional service kind filter (llm, …)"`
	Refresh string `query:"refresh" doc:"If 1 or true, import/refresh live models (extension: persist discovered; native: live ListModels)" example:"true"`
}

type ProviderModelsOutput struct {
	Body struct {
		Models              []modelInfo `json:"models" doc:"List of models for this provider"`
		DiscoverySupported  bool        `json:"discovery_supported" doc:"True when live/import discovery is available for this provider"`
		DiscoveryError      string      `json:"discovery_error,omitempty" doc:"Last discovery/import error if any (list still returned when possible)"`
		Refreshed           bool        `json:"refreshed,omitempty" doc:"True when refresh=true was requested and import ran"`
		Imported            int         `json:"imported,omitempty" doc:"Rows written on extension refresh (discovered replace)"`
	}
}

type modelInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Custom     bool   `json:"custom,omitempty"`
	DBID       string `json:"db_id,omitempty"`
	Discovered bool   `json:"discovered,omitempty"`
}

func parseRefreshFlag(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (s *Handler) HumaProviderModels(ctx context.Context, input *ProviderModelsInput) (*ProviderModelsOutput, error) {
	providerID := input.ID
	if _, ok := connectors.SpecByID(providerID); !ok {
		return nil, huma.Error404NotFound("unknown provider: " + providerID)
	}
	kindFilter := core.ServiceKind(strings.ToLower(strings.TrimSpace(input.Kind)))
	if kindFilter != "" && !core.ValidServiceKind(kindFilter) {
		return nil, huma.Error400BadRequest("unknown model kind: " + string(kindFilter))
	}
	wantRefresh := parseRefreshFlag(input.Refresh)

	modelKind := func(kind core.ServiceKind) core.ServiceKind {
		if kind != "" {
			return kind
		}
		return core.ServiceLLM
	}

	customByID := map[string]schema.CustomModel{}
	if s.db != nil {
		if cms, cerr := s.db.CustomProviders().ListModelsByProvider(ctx, providerID); cerr == nil {
			for _, cm := range cms {
				customByID[cm.ModelID] = cm
			}
		}
	}

	static := connectors.ModelsForProvider(providerID)
	seen := map[string]bool{}
	var out []modelInfo
	for _, m := range static {
		kind := modelKind(m.Kind)
		if kindFilter != "" && kind != kindFilter {
			continue
		}
		mi := modelInfo{ID: m.ID, Name: m.Name, Kind: string(kind)}
		if cm, ok := customByID[m.ID]; ok {
			mi.Custom = true
			mi.DBID = cm.ID
		}
		out = append(out, mi)
		seen[m.ID] = true
	}

	r := openapi.RequestFromContext(ctx)
	reqCtx := ctx
	if r != nil {
		reqCtx = r.Context()
	}

	liveSrc := connectors.GetLiveModelSource(providerID)
	discoverySupported := liveSrc != nil
	var discoveryError string
	refreshed := false
	imported := 0

	// Extension import: refresh=true persists via the same path as POST /extensions/{slug}/sync-models.
	if wantRefresh && s.db != nil && !connectors.IsNativeSlug(providerID) {
		ext, err := s.db.Extensions().FindBySlug(ctx, providerID)
		if err != nil {
			return nil, huma.Error404NotFound("extension not found: " + providerID)
		}
		discoverySupported = s.wasmEngine != nil
		if s.wasmEngine == nil {
			return nil, huma.Error503ServiceUnavailable("WASM engine not available")
		}
		n, syncErr := s.syncExtensionModels(ctx, ext)
		refreshed = true
		if syncErr != nil {
			discoveryError = syncErr.Error()
			s.log.WithError(syncErr).Warn("provider models refresh: list_models failed", "provider", providerID)
			// Still return catalog from DB below; UI sees discovery_error.
		} else {
			imported = n
		}
	}

	if liveSrc != nil {
		appendLive := func(creds core.Credentials) (bool, error) {
			liveCtx, cancel := context.WithTimeout(reqCtx, 10*time.Second)
			models, merr := liveSrc.ListModels(liveCtx, creds)
			cancel()
			if merr != nil {
				return false, merr
			}
			if len(models) == 0 {
				return false, nil
			}
			added := false
			for _, lm := range models {
				kind := modelKind(lm.Kind)
				if kindFilter != "" && kind != kindFilter {
					continue
				}
				if seen[lm.ID] {
					continue
				}
				out = append(out, modelInfo{ID: lm.ID, Name: lm.Name, Kind: string(kind), Discovered: true})
				seen[lm.ID] = true
				added = true
			}
			return added, nil
		}

		// Native refresh forces live probe; normal GET still tries live when accounts exist.
		if wantRefresh {
			refreshed = true
		}
		discovered := false
		var lastLiveErr error
		if s.accounts != nil && s.vault != nil {
			if accs, err := s.accounts.ListByProvider(reqCtx, adminTenant, providerID); err == nil {
				for _, acc := range accs {
					if acc.Disabled {
						continue
					}
					creds, oerr := s.vault.Open(acc)
					if oerr != nil {
						continue
					}
					added, lerr := appendLive(creds)
					if lerr != nil {
						lastLiveErr = lerr
						continue
					}
					if added {
						discovered = true
						break
					}
				}
			}
		}

		if !discovered && (wantRefresh || len(out) == 0) {
			if added, lerr := appendLive(core.Credentials{}); lerr != nil {
				lastLiveErr = lerr
			} else if added {
				discovered = true
			}
			_ = discovered
		}
		if lastLiveErr != nil && discoveryError == "" {
			discoveryError = lastLiveErr.Error()
		}
	}

	// WASM extensions: persisted DB rows only on normal GET.
	// Live list_models is imported only via refresh=true (persist), so Clear is meaningful.
	if s.db != nil && !connectors.IsNativeSlug(providerID) {
		if ext, err := s.db.Extensions().FindBySlug(ctx, providerID); err == nil {
			if s.wasmEngine != nil {
				discoverySupported = true
			}
			if ems, err := s.db.ExtensionModels().ListByExtension(ctx, ext.ID); err == nil {
				for _, em := range ems {
					modelID := strings.TrimSpace(em.Slug)
					if modelID == "" {
						if _, mid, ok := strings.Cut(em.ID, "/"); ok {
							modelID = mid
						} else {
							modelID = em.ID
						}
					}
					if modelID == "" || seen[modelID] {
						continue
					}
					if kindFilter != "" && kindFilter != core.ServiceLLM {
						continue
					}
					name := strings.TrimSpace(em.DisplayName)
					if name == "" {
						name = modelID
					}
					out = append(out, modelInfo{
						ID:         modelID,
						Name:       name,
						Kind:       string(core.ServiceLLM),
						Discovered: em.Source == "discovered",
					})
					seen[modelID] = true
				}
			}
		}
	}

	// Native with neither live source nor static/custom: not discovery-capable.
	if connectors.IsNativeSlug(providerID) && liveSrc == nil {
		discoverySupported = false
	}

	if out == nil {
		out = []modelInfo{}
	}

	return &ProviderModelsOutput{Body: struct {
		Models             []modelInfo `json:"models" doc:"List of models for this provider"`
		DiscoverySupported bool        `json:"discovery_supported" doc:"True when live/import discovery is available for this provider"`
		DiscoveryError     string      `json:"discovery_error,omitempty" doc:"Last discovery/import error if any (list still returned when possible)"`
		Refreshed          bool        `json:"refreshed,omitempty" doc:"True when refresh=true was requested and import ran"`
		Imported           int         `json:"imported,omitempty" doc:"Rows written on extension refresh (discovered replace)"`
	}{
		Models:             out,
		DiscoverySupported: discoverySupported,
		DiscoveryError:     discoveryError,
		Refreshed:          refreshed,
		Imported:           imported,
	}}, nil
}

// --- Clear discovered provider models ---

type ClearProviderDiscoveredModelsInput struct {
	// Same id as GET /providers → providers[].id
	ID      string `path:"id" doc:"Provider id from GET /providers → providers[].id (e.g. xiaomi-mimo)" example:"xiaomi-mimo"`
	ModelID string `query:"model_id" doc:"Optional specific model ID to delete (e.g. 'moonshotai/kimi-k3'). If empty, clears all discovered." example:"moonshotai/kimi-k3"`
	Source  string `query:"source" doc:"Source filter (defaults to 'discovered')" example:"discovered"`
}

type ClearProviderDiscoveredModelsOutput struct {
	Body struct {
		Provider string `json:"provider"`
		Source   string `json:"source"`
		Cleared  int    `json:"cleared" doc:"Number of discovered rows removed"`
	}
}

func (s *Handler) HumaClearProviderDiscoveredModels(ctx context.Context, input *ClearProviderDiscoveredModelsInput) (*ClearProviderDiscoveredModelsOutput, error) {
	providerID := strings.TrimSpace(input.ID)
	if _, ok := connectors.SpecByID(providerID); !ok {
		return nil, huma.Error404NotFound("unknown provider: " + providerID)
	}
	modelID := strings.TrimSpace(input.ModelID)
	source := strings.ToLower(strings.TrimSpace(input.Source))
	if source == "" {
		source = "discovered"
	}
	if connectors.IsNativeSlug(providerID) {
		// Native live models are not persisted as discovered rows yet.
		return &ClearProviderDiscoveredModelsOutput{Body: struct {
			Provider string `json:"provider"`
			Source   string `json:"source"`
			Cleared  int    `json:"cleared" doc:"Number of discovered rows removed"`
		}{Provider: providerID, Source: source, Cleared: 0}}, nil
	}
	if s.db == nil {
		return nil, huma.Error503ServiceUnavailable("database not available")
	}
	ext, err := s.db.Extensions().FindBySlug(ctx, providerID)
	if err != nil {
		return nil, huma.Error404NotFound("extension not found: " + providerID)
	}

	if modelID != "" {
		// Single model deletion
		if err := s.db.ExtensionModels().DeleteBySlug(ctx, ext.ID, modelID); err != nil {
			return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "delete model failed"))
		}
		return &ClearProviderDiscoveredModelsOutput{Body: struct {
			Provider string `json:"provider"`
			Source   string `json:"source"`
			Cleared  int    `json:"cleared" doc:"Number of discovered rows removed"`
		}{Provider: providerID, Source: source, Cleared: 1}}, nil
	}

	// Bulk clear all discovered models for this extension
	existing, err := s.db.ExtensionModels().ListBySource(ctx, ext.ID, source)
	if err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "list discovered models failed"))
	}
	if err := s.db.ExtensionModels().DeleteBySource(ctx, ext.ID, source); err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "clear discovered models failed"))
	}
	return &ClearProviderDiscoveredModelsOutput{Body: struct {
		Provider string `json:"provider"`
		Source   string `json:"source"`
		Cleared  int    `json:"cleared" doc:"Number of discovered rows removed"`
	}{Provider: providerID, Source: source, Cleared: len(existing)}}, nil
}

type BulkDeleteProviderModelsInput struct {
	ID   string `path:"id" doc:"Provider id (e.g. cline)" example:"cline"`
	Body struct {
		ModelIDs []string `json:"model_ids" doc:"List of model IDs to delete"`
	}
}

type BulkDeleteProviderModelsOutput struct {
	Body struct {
		Provider string `json:"provider"`
		Deleted  int    `json:"deleted" doc:"Number of models removed"`
	}
}

func (s *Handler) HumaBulkDeleteProviderModels(ctx context.Context, input *BulkDeleteProviderModelsInput) (*BulkDeleteProviderModelsOutput, error) {
	providerID := strings.TrimSpace(input.ID)
	if _, ok := connectors.SpecByID(providerID); !ok {
		return nil, huma.Error404NotFound("unknown provider: " + providerID)
	}
	if s.db == nil {
		return nil, huma.Error503ServiceUnavailable("database not available")
	}
	deleted := 0
	if !connectors.IsNativeSlug(providerID) {
		if ext, err := s.db.Extensions().FindBySlug(ctx, providerID); err == nil {
			for _, mid := range input.Body.ModelIDs {
				mid = strings.TrimSpace(mid)
				if mid == "" {
					continue
				}
				if err := s.db.ExtensionModels().DeleteBySlug(ctx, ext.ID, mid); err == nil {
					deleted++
				}
			}
		}
	}
	return &BulkDeleteProviderModelsOutput{Body: struct {
		Provider string `json:"provider"`
		Deleted  int    `json:"deleted" doc:"Number of models removed"`
	}{Provider: providerID, Deleted: deleted}}, nil
}

// --- Provider Routing ---

type GetProviderRoutingInput struct {
	// Same id as GET /providers → providers[].id
	ID string `path:"id" doc:"Provider id from GET /providers → providers[].id (e.g. xiaomi-mimo)" example:"xiaomi-mimo"`
}

type GetProviderRoutingOutput struct {
	Body ProviderRoutingSettings
}

func (s *Handler) HumaGetProviderRouting(ctx context.Context, input *GetProviderRoutingInput) (*GetProviderRoutingOutput, error) {
	provider := input.ID
	if _, ok := connectors.SpecByID(provider); !ok {
		return nil, huma.Error404NotFound("provider not found")
	}
	return &GetProviderRoutingOutput{Body: s.loadProviderRoutingSettings(ctx, provider)}, nil
}

type UpdateProviderRoutingInput struct {
	ID   string `path:"id" doc:"Provider id from GET /providers → providers[].id (e.g. xiaomi-mimo)" example:"xiaomi-mimo"`
	Body struct {
		RoutingStrategy    *string `json:"routing_strategy,omitempty" doc:"inherit | fill-first | round-robin | smart-round-robin"`
		StickyLimit        *int    `json:"sticky_limit,omitempty" doc:"Sticky limit"`
		AffinityTTLMinutes *int    `json:"affinity_ttl_minutes,omitempty" doc:"Affinity TTL in minutes"`
	}
}

type UpdateProviderRoutingOutput struct {
	Body ProviderRoutingSettings
}

func (s *Handler) HumaUpdateProviderRouting(ctx context.Context, input *UpdateProviderRoutingInput) (*UpdateProviderRoutingOutput, error) {
	if s.settings == nil {
		return nil, huma.Error500InternalServerError("settings store not configured")
	}
	provider := input.ID
	if _, ok := connectors.SpecByID(provider); !ok {
		return nil, huma.Error404NotFound("provider not found")
	}
	current := s.loadProviderRoutingSettings(ctx, provider)
	if input.Body.RoutingStrategy != nil {
		normalized, ok := normalizeProviderRoutingStrategy(*input.Body.RoutingStrategy)
		if !ok {
			return nil, huma.Error400BadRequest("routing_strategy must be inherit, fill-first, round-robin, or smart-round-robin")
		}
		current.RoutingStrategy = normalized
	}
	if input.Body.StickyLimit != nil {
		if *input.Body.StickyLimit < 1 {
			return nil, huma.Error400BadRequest("sticky_limit must be at least 1")
		}
		current.StickyLimit = *input.Body.StickyLimit
	}
	if input.Body.AffinityTTLMinutes != nil {
		if *input.Body.AffinityTTLMinutes < 1 {
			return nil, huma.Error400BadRequest("affinity_ttl_minutes must be at least 1")
		}
		current.AffinityTTLMinutes = *input.Body.AffinityTTLMinutes
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	if err := s.settings.Set(ctx, providerRoutingPrefix+provider, string(raw)); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &UpdateProviderRoutingOutput{Body: current}, nil
}

// --- Get Endpoint Settings ---

type GetEndpointSettingsInput struct{}

type GetEndpointSettingsOutput struct {
	Body EndpointSettings
}

func (s *Handler) HumaGetEndpointSettings(ctx context.Context, _ *GetEndpointSettingsInput) (*GetEndpointSettingsOutput, error) {
	return &GetEndpointSettingsOutput{Body: s.loadEndpointSettings(ctx)}, nil
}

// --- Update Endpoint Settings ---

type UpdateEndpointSettingsInput struct {
	Body struct {
		RTKEnabled     *bool   `json:"rtk_enabled,omitempty"`
		RTKFilterLevel *string `json:"rtk_filter_level,omitempty"`
		CavemanEnabled *bool   `json:"caveman_enabled,omitempty"`
		CavemanLevel   *string `json:"caveman_level,omitempty"`
		TerseEnabled   *bool   `json:"terse_enabled,omitempty"`
		TerseLevel     *string `json:"terse_level,omitempty"`

		HeadroomEnabled              *bool   `json:"headroom_enabled,omitempty"`
		HeadroomURL                  *string `json:"headroom_url,omitempty"`
		HeadroomCompressUserMessages *bool   `json:"headroom_compress_user_messages,omitempty"`
		HeadroomTimeoutMs            *int    `json:"headroom_timeout_ms,omitempty"`
		PonytailEnabled              *bool   `json:"ponytail_enabled,omitempty"`
		PonytailLevel                *string `json:"ponytail_level,omitempty"`

		RoutingStrategy         *string `json:"routing_strategy,omitempty"`
		StickyLimit             *int    `json:"sticky_limit,omitempty"`
		ComboStrategy           *string `json:"combo_strategy,omitempty"`
		ComboStickyLimit        *int    `json:"combo_sticky_limit,omitempty"`
		OutboundProxyEnabled    *bool   `json:"outbound_proxy_enabled,omitempty"`
		OutboundProxyURL        *string `json:"outbound_proxy_url,omitempty"`
		OutboundNoProxy         *string `json:"outbound_no_proxy,omitempty"`
		ObservabilityEnabled    *bool   `json:"observability_enabled,omitempty"`
		RateLimitsEnabled       *bool   `json:"rate_limits_enabled,omitempty"`
		StreamStallTimeoutMs    *int    `json:"stream_stall_timeout_ms,omitempty"`
		ResponseHeaderTimeoutMs *int    `json:"response_header_timeout_ms,omitempty"`
		RequestTimeoutMs        *int    `json:"request_timeout_ms,omitempty"`
	}
}

type UpdateEndpointSettingsOutput struct {
	Body EndpointSettings
}

func (s *Handler) HumaUpdateEndpointSettings(ctx context.Context, input *UpdateEndpointSettingsInput) (*UpdateEndpointSettingsOutput, error) {
	if s.settings == nil {
		return nil, huma.Error500InternalServerError("settings store not configured")
	}
	current := s.loadEndpointSettings(ctx)

	if input.Body.RTKEnabled != nil {
		current.RTKEnabled = *input.Body.RTKEnabled
	}
	if input.Body.RTKFilterLevel != nil {
		switch *input.Body.RTKFilterLevel {
		case "none", "minimal", "aggressive":
			current.RTKFilterLevel = *input.Body.RTKFilterLevel
		default:
			return nil, huma.Error400BadRequest("rtk_filter_level must be none, minimal, or aggressive")
		}
	}
	if input.Body.CavemanEnabled != nil {
		current.CavemanEnabled = *input.Body.CavemanEnabled
	}
	if input.Body.CavemanLevel != nil {
		if !caveman.ValidLevel(caveman.Level(*input.Body.CavemanLevel)) {
			return nil, huma.Error400BadRequest("caveman_level must be lite, full, ultra, wenyan-lite, wenyan-full, or wenyan-ultra")
		}
		current.CavemanLevel = *input.Body.CavemanLevel
	}
	if input.Body.TerseEnabled != nil {
		current.TerseEnabled = *input.Body.TerseEnabled
	}
	if input.Body.TerseLevel != nil {
		if !terse.ValidLevel(terse.Level(*input.Body.TerseLevel)) {
			return nil, huma.Error400BadRequest("terse_level must be light, medium, or aggressive")
		}
		current.TerseLevel = *input.Body.TerseLevel
	}
	if input.Body.HeadroomEnabled != nil {
		current.HeadroomEnabled = *input.Body.HeadroomEnabled
	}
	if input.Body.HeadroomURL != nil {
		current.HeadroomURL = *input.Body.HeadroomURL
	}
	if input.Body.HeadroomCompressUserMessages != nil {
		current.HeadroomCompressUserMessages = *input.Body.HeadroomCompressUserMessages
	}
	if input.Body.HeadroomTimeoutMs != nil {
		if *input.Body.HeadroomTimeoutMs < 1000 || *input.Body.HeadroomTimeoutMs > 60000 {
			return nil, huma.Error400BadRequest("headroom_timeout_ms must be between 1000 and 60000 ms")
		}
		current.HeadroomTimeoutMs = *input.Body.HeadroomTimeoutMs
	}
	if input.Body.PonytailEnabled != nil {
		current.PonytailEnabled = *input.Body.PonytailEnabled
	}
	if input.Body.PonytailLevel != nil {
		if !ponytail.ValidLevel(ponytail.Level(*input.Body.PonytailLevel)) {
			return nil, huma.Error400BadRequest("ponytail_level must be lite, full, or ultra")
		}
		current.PonytailLevel = *input.Body.PonytailLevel
	}
	if current.HeadroomEnabled && strings.TrimSpace(current.HeadroomURL) == "" {
		return nil, huma.Error400BadRequest("headroom_url is required when Headroom is enabled")
	}
	if input.Body.RoutingStrategy != nil {
		normalized, ok := normalizeAccountRoutingStrategy(*input.Body.RoutingStrategy)
		if !ok {
			return nil, huma.Error400BadRequest("routing_strategy must be fill-first, round-robin, or smart-round-robin")
		}
		current.RoutingStrategy = normalized
	}
	if input.Body.StickyLimit != nil {
		if *input.Body.StickyLimit < 1 {
			return nil, huma.Error400BadRequest("sticky_limit must be at least 1")
		}
		current.StickyLimit = *input.Body.StickyLimit
	}
	if input.Body.ComboStrategy != nil {
		normalized, ok := normalizeComboRoutingStrategy(*input.Body.ComboStrategy)
		if !ok {
			return nil, huma.Error400BadRequest("combo_strategy must be fallback or round-robin")
		}
		current.ComboStrategy = normalized
	}
	if input.Body.ComboStickyLimit != nil {
		if *input.Body.ComboStickyLimit < 1 {
			return nil, huma.Error400BadRequest("combo_sticky_limit must be at least 1")
		}
		current.ComboStickyLimit = *input.Body.ComboStickyLimit
	}
	if input.Body.OutboundProxyEnabled != nil {
		current.OutboundProxyEnabled = *input.Body.OutboundProxyEnabled
	}
	if input.Body.OutboundProxyURL != nil {
		current.OutboundProxyURL = *input.Body.OutboundProxyURL
	}
	if input.Body.OutboundNoProxy != nil {
		current.OutboundNoProxy = *input.Body.OutboundNoProxy
	}
	if input.Body.ObservabilityEnabled != nil {
		current.ObservabilityEnabled = input.Body.ObservabilityEnabled
	}
	if input.Body.RateLimitsEnabled != nil {
		current.RateLimitsEnabled = *input.Body.RateLimitsEnabled
	}
	if input.Body.StreamStallTimeoutMs != nil {
		if *input.Body.StreamStallTimeoutMs < 5000 || *input.Body.StreamStallTimeoutMs > 600000 {
			return nil, huma.Error400BadRequest("stream_stall_timeout_ms must be between 5000 and 600000")
		}
		current.StreamStallTimeoutMs = *input.Body.StreamStallTimeoutMs
	}
	if input.Body.ResponseHeaderTimeoutMs != nil {
		if *input.Body.ResponseHeaderTimeoutMs < 5000 || *input.Body.ResponseHeaderTimeoutMs > 300000 {
			return nil, huma.Error400BadRequest("response_header_timeout_ms must be between 5000 and 300000")
		}
		current.ResponseHeaderTimeoutMs = *input.Body.ResponseHeaderTimeoutMs
	}
	if input.Body.RequestTimeoutMs != nil {
		if *input.Body.RequestTimeoutMs < 30000 || *input.Body.RequestTimeoutMs > 3600000 {
			return nil, huma.Error400BadRequest("request_timeout_ms must be between 30000 and 3600000")
		}
		current.RequestTimeoutMs = *input.Body.RequestTimeoutMs
	}

	if current.CavemanEnabled && current.TerseEnabled {
		if input.Body.TerseEnabled != nil && *input.Body.TerseEnabled {
			current.CavemanEnabled = false
		} else {
			current.TerseEnabled = false
		}
	}

	raw, err := json.Marshal(current)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	if err := s.settings.Set(ctx, endpointSettingsKey, string(raw)); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}

	if s.timeoutNotifier != nil {
		s.timeoutNotifier.NotifyTimeouts(
			time.Duration(current.StreamStallTimeoutMs)*time.Millisecond,
			time.Duration(current.ResponseHeaderTimeoutMs)*time.Millisecond,
			time.Duration(current.RequestTimeoutMs)*time.Millisecond,
		)
	}

	if s.proxyNotifier != nil {
		s.proxyNotifier.NotifyProxy(
			current.OutboundProxyEnabled,
			current.OutboundProxyURL,
			current.OutboundNoProxy,
		)
	}

	if s.rateLimiter != nil {
		s.rateLimiter.SetEnabled(current.RateLimitsEnabled)
	}

	return &UpdateEndpointSettingsOutput{Body: current}, nil
}

// --- Test Headroom ---

type TestHeadroomInput struct {
	Body struct {
		URL       *string `json:"url,omitempty"`
		TimeoutMs *int    `json:"timeout_ms,omitempty"`
	}
}

type TestHeadroomOutput struct {
	Body struct {
		OK        bool   `json:"ok"`
		Reachable bool   `json:"reachable"`
		Status    int    `json:"status"`
		LatencyMs int64  `json:"latency_ms"`
		Endpoint  string `json:"endpoint"`
		Message   string `json:"message"`
	}
}

func (s *Handler) HumaTestHeadroom(ctx context.Context, input *TestHeadroomInput) (*TestHeadroomOutput, error) {
	es := s.loadEndpointSettings(ctx)

	url := es.HeadroomURL
	if input.Body.URL != nil {
		url = *input.Body.URL
	}
	if strings.TrimSpace(url) == "" {
		return nil, huma.Error400BadRequest("headroom_url is required to test the connection")
	}

	timeoutMs := es.HeadroomTimeoutMs
	if input.Body.TimeoutMs != nil {
		timeoutMs = *input.Body.TimeoutMs
	}
	if timeoutMs < 1000 || timeoutMs > 60000 {
		return nil, huma.Error400BadRequest("headroom_timeout_ms must be between 1000 and 60000 ms")
	}

	result := headroom.New(nil).Probe(ctx, headroom.Config{
		Enabled: true,
		URL:     url,
		Timeout: time.Duration(timeoutMs) * time.Millisecond,
	})
	return &TestHeadroomOutput{Body: struct {
		OK        bool   `json:"ok"`
		Reachable bool   `json:"reachable"`
		Status    int    `json:"status"`
		LatencyMs int64  `json:"latency_ms"`
		Endpoint  string `json:"endpoint"`
		Message   string `json:"message"`
	}{OK: result.OK, Reachable: result.Reachable, Status: result.Status, LatencyMs: result.LatencyMs, Endpoint: result.Endpoint, Message: result.Message}}, nil
}

// --- Get Access Settings ---

type GetAccessSettingsInput struct{}

type GetAccessSettingsOutput struct {
	Body map[string]any
}

func (s *Handler) HumaGetAccessSettings(ctx context.Context, _ *GetAccessSettingsInput) (*GetAccessSettingsOutput, error) {
	r := openapi.RequestFromContext(ctx)
	as := s.loadAccessSettings(ctx)
	return &GetAccessSettingsOutput{Body: map[string]any{
		"local_enabled":     as.LocalEnabled,
		"tunnel_enabled":    as.TunnelEnabled,
		"tailscale_enabled": as.Tailscale,
		"tunnel_url":        as.TunnelURL,
		"tailscale_url":     as.TailscaleURL,
		"endpoint_url":      s.publicBaseURL(r) + "/v1",
	}}, nil
}

// --- Update Access Settings ---

type UpdateAccessSettingsInput struct {
	Body struct {
		LocalEnabled  *bool `json:"local_enabled,omitempty"`
		TunnelEnabled *bool `json:"tunnel_enabled,omitempty"`
		Tailscale     *bool `json:"tailscale_enabled,omitempty"`
	}
}

type UpdateAccessSettingsOutput struct {
	Body map[string]any
}

func (s *Handler) HumaUpdateAccessSettings(ctx context.Context, input *UpdateAccessSettingsInput) (*UpdateAccessSettingsOutput, error) {
	if s.settings == nil {
		return nil, huma.Error500InternalServerError("settings store not configured")
	}
	r := openapi.RequestFromContext(ctx)
	current := s.loadAccessSettings(ctx)

	if input.Body.LocalEnabled != nil {
		current.LocalEnabled = *input.Body.LocalEnabled
	}
	if input.Body.TunnelEnabled != nil {
		current.TunnelEnabled = *input.Body.TunnelEnabled
	}
	if input.Body.Tailscale != nil {
		current.Tailscale = *input.Body.Tailscale
	}

	raw, err := json.Marshal(current)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	if err := s.settings.Set(ctx, accessSettingsKey, string(raw)); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &UpdateAccessSettingsOutput{Body: map[string]any{
		"local_enabled":     current.LocalEnabled,
		"tunnel_enabled":    current.TunnelEnabled,
		"tailscale_enabled": current.Tailscale,
		"endpoint_url":      s.publicBaseURL(r) + "/v1",
	}}, nil
}

// --- List Aliases ---

type ListAliasesInput struct{}

type ListAliasesOutput struct {
	Body struct {
		Aliases map[string]string `json:"aliases" doc:"Map of alias to target model"`
	}
}

func (s *Handler) HumaListAliases(ctx context.Context, _ *ListAliasesInput) (*ListAliasesOutput, error) {
	aliases, err := s.aliases.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "internal server error"))
	}
	out := make(map[string]string, len(aliases))
	for _, a := range aliases {
		out[a.Alias] = a.Target
	}
	return &ListAliasesOutput{Body: struct {
		Aliases map[string]string `json:"aliases" doc:"Map of alias to target model"`
	}{Aliases: out}}, nil
}

// --- Set Alias ---

type SetAliasInput struct {
	Body struct {
		Alias  string `json:"alias" doc:"Alias name"`
		Target string `json:"target" doc:"Target model in provider/model format"`
	}
}

type SetAliasOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (s *Handler) HumaSetAlias(ctx context.Context, input *SetAliasInput) (*SetAliasOutput, error) {
	if input.Body.Alias == "" || input.Body.Target == "" {
		return nil, huma.Error400BadRequest("alias and target are required")
	}
	if !strings.Contains(input.Body.Target, "/") {
		return nil, huma.Error400BadRequest("target must be in 'provider/model' format")
	}
	if err := s.aliases.Set(ctx, input.Body.Alias, input.Body.Target); err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "internal server error"))
	}
	return &SetAliasOutput{Body: struct {
		OK bool `json:"ok"`
	}{OK: true}}, nil
}

// --- Delete Alias ---

type DeleteAliasInput struct {
	Alias string `query:"alias" required:"true" doc:"Alias to delete"`
}

type DeleteAliasOutput struct {
	Body struct{}
}

func (s *Handler) HumaDeleteAlias(ctx context.Context, input *DeleteAliasInput) (*DeleteAliasOutput, error) {
	if input.Alias == "" {
		return nil, huma.Error400BadRequest("alias query param is required")
	}
	if err := s.aliases.Delete(ctx, input.Alias); err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "internal server error"))
	}
	return &DeleteAliasOutput{}, nil
}

// --- List Disabled Models ---

type ListDisabledModelsInput struct {
	Provider string `query:"provider" required:"true" doc:"Provider alias"`
}

type ListDisabledModelsOutput struct {
	Body struct {
		IDs []string `json:"ids" doc:"List of disabled model IDs"`
	}
}

func (s *Handler) HumaListDisabledModels(ctx context.Context, input *ListDisabledModelsInput) (*ListDisabledModelsOutput, error) {
	if input.Provider == "" {
		return nil, huma.Error400BadRequest("provider query param is required")
	}
	ids := s.loadDisabledModels(ctx, input.Provider)
	return &ListDisabledModelsOutput{Body: struct {
		IDs []string `json:"ids" doc:"List of disabled model IDs"`
	}{IDs: ids}}, nil
}

// --- Disable Models ---

type DisableModelsInput struct {
	Body struct {
		Provider string   `json:"providerAlias" doc:"Provider alias"`
		IDs      []string `json:"ids" doc:"Model IDs to disable"`
	}
}

type DisableModelsOutput struct {
	Body struct {
		IDs []string `json:"ids" doc:"All disabled model IDs after update"`
	}
}

func (s *Handler) HumaDisableModels(ctx context.Context, input *DisableModelsInput) (*DisableModelsOutput, error) {
	if input.Body.Provider == "" {
		return nil, huma.Error400BadRequest("providerAlias is required")
	}
	existing := s.loadDisabledModels(ctx, input.Body.Provider)
	seen := map[string]bool{}
	for _, id := range existing {
		seen[id] = true
	}
	for _, id := range input.Body.IDs {
		seen[id] = true
	}
	merged := make([]string, 0, len(seen))
	for id := range seen {
		merged = append(merged, id)
	}
	if err := s.saveDisabledModels(ctx, input.Body.Provider, merged); err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "internal server error"))
	}
	return &DisableModelsOutput{Body: struct {
		IDs []string `json:"ids" doc:"All disabled model IDs after update"`
	}{IDs: merged}}, nil
}

// --- Enable Models ---

type EnableModelsInput struct {
	Body struct {
		Provider string   `json:"providerAlias" doc:"Provider alias"`
		IDs      []string `json:"ids" doc:"Model IDs to enable"`
	}
}

type EnableModelsOutput struct {
	Body struct {
		IDs []string `json:"ids" doc:"Remaining disabled model IDs after update"`
	}
}

func (s *Handler) HumaEnableModels(ctx context.Context, input *EnableModelsInput) (*EnableModelsOutput, error) {
	if input.Body.Provider == "" {
		return nil, huma.Error400BadRequest("providerAlias is required")
	}
	existing := s.loadDisabledModels(ctx, input.Body.Provider)
	remove := map[string]bool{}
	for _, id := range input.Body.IDs {
		remove[id] = true
	}
	var kept []string
	for _, id := range existing {
		if !remove[id] {
			kept = append(kept, id)
		}
	}
	if err := s.saveDisabledModels(ctx, input.Body.Provider, kept); err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "internal server error"))
	}
	return &EnableModelsOutput{Body: struct {
		IDs []string `json:"ids" doc:"Remaining disabled model IDs after update"`
	}{IDs: kept}}, nil
}

// --- List Guardrails ---

type ListGuardrailsInput struct {
	Scope string `query:"scope" doc:"Scope filter (global, provider, model, chain, apikey)"`
}

type ListGuardrailsOutput struct {
	Body struct {
		Guardrails []guardrailDTO `json:"guardrails"`
	}
}

func (s *Handler) HumaListGuardrails(ctx context.Context, input *ListGuardrailsInput) (*ListGuardrailsOutput, error) {
	scope := schema.GuardrailScope(strings.TrimSpace(input.Scope))
	rows, err := s.guardrailRepo.List(ctx, schema.DefaultTenantID, string(scope))
	if err != nil {
		return nil, huma.Error500InternalServerError("list guardrails: " + err.Error())
	}
	out := make([]guardrailDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, toDTO(p))
	}
	return &ListGuardrailsOutput{Body: struct {
		Guardrails []guardrailDTO `json:"guardrails"`
	}{Guardrails: out}}, nil
}

// --- Get Guardrail ---

type GetGuardrailInput struct {
	ID string `path:"id" doc:"Guardrail ID"`
}

type GetGuardrailOutput struct {
	Body guardrailDTO
}

func (s *Handler) HumaGetGuardrail(ctx context.Context, input *GetGuardrailInput) (*GetGuardrailOutput, error) {
	p, err := s.guardrailRepo.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("guardrail not found")
	}
	return &GetGuardrailOutput{Body: toDTO(p)}, nil
}

// --- Create Guardrail ---

type CreateGuardrailInput struct {
	Body struct {
		Name    string             `json:"name"`
		Scope   string             `json:"scope"`
		ScopeID string             `json:"scope_id"`
		Enabled *bool              `json:"enabled,omitempty"`
		Config  *guardrails.Policy `json:"config,omitempty"`
	}
}

type CreateGuardrailOutput struct {
	Body guardrailDTO
}

func (s *Handler) HumaCreateGuardrail(ctx context.Context, input *CreateGuardrailInput) (*CreateGuardrailOutput, error) {
	scope := schema.GuardrailScope(strings.TrimSpace(input.Body.Scope))
	if !isValidScope(scope) {
		return nil, huma.Error400BadRequest("invalid scope: " + input.Body.Scope)
	}
	scopeID := strings.TrimSpace(input.Body.ScopeID)
	if scope == schema.GuardrailScopeGlobal {
		scopeID = ""
	} else if scopeID == "" {
		return nil, huma.Error400BadRequest("scope_id required for non-global scope")
	}
	name := strings.TrimSpace(input.Body.Name)
	if name == "" {
		name = defaultPolicyName(scope, scopeID)
	}

	cfg := guardrails.Policy{}
	if input.Body.Config != nil {
		cfg = *input.Body.Config
	}
	cfgJSON, err := guardrails.MarshalPolicy(cfg)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid config: " + err.Error())
	}
	enabled := true
	if input.Body.Enabled != nil {
		enabled = *input.Body.Enabled
	}
	now := time.Now().UTC()
	p := schema.GuardrailPolicy{
		ID:        newGuardrailID(),
		TenantID:  schema.DefaultTenantID,
		Scope:     string(scope),
		ScopeID:   scopeID,
		Name:      name,
		Enabled:   enabled,
		Config:    cfgJSON,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.guardrailRepo.Upsert(ctx, p); err != nil {
		return nil, huma.Error500InternalServerError("save guardrail: " + err.Error())
	}
	s.invalidateGuardrail(p.TenantID, schema.GuardrailScope(p.Scope), p.ScopeID)

	final, err := s.guardrailRepo.GetByScope(ctx, schema.DefaultTenantID, scope, scopeID)
	if err != nil {
		final = p
	}
	return &CreateGuardrailOutput{Body: toDTO(final)}, nil
}

// --- Update Guardrail ---

type UpdateGuardrailInput struct {
	ID   string `path:"id" doc:"Guardrail ID"`
	Body struct {
		Name    string             `json:"name,omitempty"`
		Enabled *bool              `json:"enabled,omitempty"`
		Config  *guardrails.Policy `json:"config,omitempty"`
	}
}

type UpdateGuardrailOutput struct {
	Body guardrailDTO
}

func (s *Handler) HumaUpdateGuardrail(ctx context.Context, input *UpdateGuardrailInput) (*UpdateGuardrailOutput, error) {
	existing, err := s.guardrailRepo.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("guardrail not found")
	}
	if strings.TrimSpace(input.Body.Name) != "" {
		existing.Name = input.Body.Name
	}
	if input.Body.Enabled != nil {
		existing.Enabled = *input.Body.Enabled
	}
	if input.Body.Config != nil {
		cfgJSON, err := guardrails.MarshalPolicy(*input.Body.Config)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid config: " + err.Error())
		}
		existing.Config = cfgJSON
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := s.guardrailRepo.Upsert(ctx, existing); err != nil {
		return nil, huma.Error500InternalServerError("save guardrail: " + err.Error())
	}
	s.invalidateGuardrail(existing.TenantID, schema.GuardrailScope(existing.Scope), existing.ScopeID)
	return &UpdateGuardrailOutput{Body: toDTO(existing)}, nil
}

// --- Delete Guardrail ---

type DeleteGuardrailInput struct {
	ID string `path:"id" doc:"Guardrail ID"`
}

type DeleteGuardrailOutput struct{}

func (s *Handler) HumaDeleteGuardrail(ctx context.Context, input *DeleteGuardrailInput) (*DeleteGuardrailOutput, error) {
	existing, err := s.guardrailRepo.Get(ctx, input.ID)
	if err == nil {
		s.invalidateGuardrail(existing.TenantID, schema.GuardrailScope(existing.Scope), existing.ScopeID)
	}
	if err := s.guardrailRepo.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError("delete guardrail: " + err.Error())
	}
	w := openapi.ResponseWriterFromContext(ctx)
	w.WriteHeader(http.StatusNoContent)
	return &DeleteGuardrailOutput{}, nil
}

// --- Effective Guardrail ---

type EffectiveGuardrailInput struct {
	Provider string `query:"provider" doc:"Provider"`
	Model    string `query:"model" doc:"Model"`
	Chain    string `query:"chain" doc:"Chain ID"`
	APIKey   string `query:"apikey" doc:"API Key ID"`
}

type EffectiveGuardrailOutput struct {
	Body struct {
		Scope  guardrails.Key    `json:"scope"`
		Policy guardrails.Policy `json:"policy"`
	}
}

func (s *Handler) HumaEffectiveGuardrail(ctx context.Context, input *EffectiveGuardrailInput) (*EffectiveGuardrailOutput, error) {
	key := guardrails.Key{
		TenantID: schema.DefaultTenantID,
		Provider: input.Provider,
		Model:    input.Model,
		ChainID:  input.Chain,
		APIKeyID: input.APIKey,
	}
	policy := s.guardrails.EffectivePolicy(ctx, key)
	return &EffectiveGuardrailOutput{Body: struct {
		Scope  guardrails.Key    `json:"scope"`
		Policy guardrails.Policy `json:"policy"`
	}{Scope: key, Policy: policy}}, nil
}

// --- List Entities ---

type ListGuardrailEntitiesInput struct{}

type ListGuardrailEntitiesOutput struct {
	Body struct {
		Entities []string `json:"entities"`
	}
}

func (s *Handler) HumaListGuardrailEntities(ctx context.Context, _ *ListGuardrailEntitiesInput) (*ListGuardrailEntitiesOutput, error) {
	entities := pii.AllEntities()
	out := make([]string, len(entities))
	for i, e := range entities {
		out[i] = string(e)
	}
	return &ListGuardrailEntitiesOutput{Body: struct {
		Entities []string `json:"entities"`
	}{Entities: out}}, nil
}

// --- List Guardrail Logs ---

type ListGuardrailLogsInput struct {
	APIKeyID string `query:"api_key_id" doc:"API Key ID filter"`
	Detector string `query:"detector" doc:"Detector filter"`
	Action   string `query:"action" doc:"Action filter"`
	Limit    string `query:"limit" doc:"Limit"`
}

type ListGuardrailLogsOutput struct {
	Body struct {
		Logs []map[string]any `json:"logs"`
	}
}

func (s *Handler) HumaListGuardrailLogs(ctx context.Context, input *ListGuardrailLogsInput) (*ListGuardrailLogsOutput, error) {
	if s.guardrailLogs == nil {
		return &ListGuardrailLogsOutput{Body: struct {
			Logs []map[string]any `json:"logs"`
		}{Logs: []map[string]any{}}}, nil
	}
	f := schema.GuardrailLogFilter{
		APIKeyID: input.APIKeyID,
		Detector: input.Detector,
		Action:   input.Action,
	}
	if input.Limit != "" {
		var n int
		if _, err := parseIntInto(input.Limit, &n); err != nil {
			return nil, huma.Error400BadRequest("invalid limit: " + err.Error())
		}
		f.Limit = n
	}
	rows, err := s.guardrailLogs.List(ctx, schema.DefaultTenantID, f)
	if err != nil {
		return nil, huma.Error500InternalServerError("list guardrail logs: " + err.Error())
	}
	out := make([]map[string]any, 0, len(rows))
	for _, e := range rows {
		var findings any
		_ = json.Unmarshal([]byte(e.Findings), &findings)
		out = append(out, map[string]any{
			"id":         e.ID,
			"request_id": e.RequestID,
			"api_key_id": e.APIKeyID,
			"provider":   e.Provider,
			"model":      e.Model,
			"chain_id":   e.ChainID,
			"detector":   e.Detector,
			"direction":  e.Direction,
			"action":     e.Action,
			"severity":   e.Severity,
			"reason":     e.Reason,
			"findings":   findings,
			"created_at": e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return &ListGuardrailLogsOutput{Body: struct {
		Logs []map[string]any `json:"logs"`
	}{Logs: out}}, nil
}

// --- Test Guardrail ---

type TestGuardrailInput struct {
	Body struct {
		Text   string             `json:"text"`
		Config *guardrails.Policy `json:"config,omitempty"`
	}
}

type TestGuardrailOutput struct {
	Body struct {
		Action    string                `json:"action"`
		Reason    string                `json:"reason"`
		Decisions []guardrails.Decision `json:"decisions"`
	}
}

func (s *Handler) HumaTestGuardrail(ctx context.Context, input *TestGuardrailInput) (*TestGuardrailOutput, error) {
	r := openapi.RequestFromContext(ctx)
	w := openapi.ResponseWriterFromContext(ctx)

	if s.guardrailTestRL != nil {
		if !s.guardrailTestRL.allow(rateLimitKeyFor(r)) {
			return nil, huma.Error429TooManyRequests("guardrails test rate-limit exceeded; try again in a minute")
		}
	}
	if input.Body.Text == "" {
		return nil, huma.Error400BadRequest("text is required")
	}

	req := &core.ChatRequest{
		Metadata: core.RequestMetadata{
			TenantID:  schema.DefaultTenantID,
			APIKeyID:  "test-panel",
			RequestID: "test-" + newGuardrailID()[3:11],
		},
		Messages: []core.Message{
			{Role: core.RoleUser, Content: []core.ContentPart{{Type: core.PartText, Text: input.Body.Text}}},
		},
	}
	if input.Body.Config != nil {
		s.runOneOffGuardrails(w, ctx, req, *input.Body.Config)
		return nil, nil
	}
	res := s.guardrails.Inbound(ctx, req)
	return &TestGuardrailOutput{Body: struct {
		Action    string                `json:"action"`
		Reason    string                `json:"reason"`
		Decisions []guardrails.Decision `json:"decisions"`
	}{Action: string(res.Action), Reason: res.Reason, Decisions: res.Decisions}}, nil
}

// --- System Status ---

type SystemStatusInput struct{}

type SystemStatusOutput struct {
	Body SystemSnapshot
}

func (s *Handler) HumaSystemStatus(ctx context.Context, _ *SystemStatusInput) (*SystemStatusOutput, error) {
	snap := collectFullSnapshot()
	return &SystemStatusOutput{Body: snap}, nil
}

// --- System History ---

type SystemHistoryInput struct{}

type SystemHistoryOutput struct {
	Body struct {
		Interval int            `json:"interval_sec"`
		MaxSize  int            `json:"max_size"`
		Spikes   []SystemSample `json:"spikes"`
		Samples  []SystemSample `json:"samples"`
	}
}

func (s *Handler) HumaSystemHistory(ctx context.Context, _ *SystemHistoryInput) (*SystemHistoryOutput, error) {
	samples := sysHistory.samples()

	spikes := make([]SystemSample, 0)
	for _, sample := range samples {
		if sample.IsCPUSpike || sample.IsMemSpike {
			spikes = append(spikes, sample)
		}
	}

	return &SystemHistoryOutput{Body: struct {
		Interval int            `json:"interval_sec"`
		MaxSize  int            `json:"max_size"`
		Spikes   []SystemSample `json:"spikes"`
		Samples  []SystemSample `json:"samples"`
	}{
		Interval: int(sampleInterval.Seconds()),
		MaxSize:  historySize,
		Spikes:   spikes,
		Samples:  samples,
	}}, nil
}

// --- System Resources ---

type SystemResourcesInput struct {
	Hours    string `query:"hours" doc:"Hours to look back (default: 24)"`
	Interval string `query:"interval" doc:"Bucket interval (default: 5m)"`
}

type SystemResourcesOutput struct {
	Body []schema.ResourceBucket
}

func (s *Handler) HumaSystemResources(ctx context.Context, input *SystemResourcesInput) (*SystemResourcesOutput, error) {
	if s.resources == nil {
		return &SystemResourcesOutput{Body: []schema.ResourceBucket{}}, nil
	}
	hours := 24
	if input.Hours != "" {
		if v, err := time.ParseDuration(input.Hours + "h"); err == nil && v > 0 {
			hours = int(v.Hours())
		}
	}
	interval := 5 * time.Minute
	if input.Interval != "" {
		if d, err := time.ParseDuration(input.Interval); err == nil && d >= time.Minute {
			interval = d
		}
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	buckets, err := s.resources.ResourceBuckets(ctx, since, interval)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to load resource history")
	}
	if buckets == nil {
		buckets = []schema.ResourceBucket{}
	}
	return &SystemResourcesOutput{Body: buckets}, nil
}

// --- Update Check ---

type UpdateCheckInput struct {
	Refresh string `query:"refresh" doc:"Force refresh (1 or true)"`
}

type UpdateCheckOutput struct {
	Body update.Info
}

func (s *Handler) HumaUpdateCheck(ctx context.Context, input *UpdateCheckInput) (*UpdateCheckOutput, error) {
	if s.updates == nil {
		return &UpdateCheckOutput{Body: update.Info{Current: s.versionString(), Checked: false}}, nil
	}
	var info *update.Info
	if input.Refresh == "1" || input.Refresh == "true" {
		info = s.updates.Refresh(ctx)
	} else {
		info = s.updates.Check(ctx)
	}
	return &UpdateCheckOutput{Body: *info}, nil
}

// --- Tunnel Status ---

type TunnelStatusInput struct{}

type TunnelStatusOutput struct {
	Body struct {
		Tunnel    any `json:"tunnel"`
		Tailscale any `json:"tailscale"`
		Download  struct {
			Downloading bool `json:"downloading"`
			Progress    int  `json:"progress"`
		} `json:"download"`
	}
}

func (s *Handler) HumaTunnelStatus(ctx context.Context, _ *TunnelStatusInput) (*TunnelStatusOutput, error) {
	tunnelStatus := s.cfManager.Status()
	tailscaleStatus := s.tsManager.Status()
	downloading, progress := cloudflare.GetDownloadStatus()

	return &TunnelStatusOutput{Body: struct {
		Tunnel    any `json:"tunnel"`
		Tailscale any `json:"tailscale"`
		Download  struct {
			Downloading bool `json:"downloading"`
			Progress    int  `json:"progress"`
		} `json:"download"`
	}{
		Tunnel:    tunnelStatus,
		Tailscale: tailscaleStatus,
		Download: struct {
			Downloading bool `json:"downloading"`
			Progress    int  `json:"progress"`
		}{Downloading: downloading, Progress: progress},
	}}, nil
}

// --- Tunnel Enable ---

type TunnelEnableInput struct{}

type TunnelEnableOutput struct {
	Body any
}

func (s *Handler) HumaTunnelEnable(ctx context.Context, _ *TunnelEnableInput) (*TunnelEnableOutput, error) {
	result, err := s.cfManager.Enable(func(tunnelURL string) {
		current := s.loadAccessSettings(ctx)
		current.TunnelEnabled = true
		raw, _ := json.Marshal(current)
		s.settings.Set(ctx, accessSettingsKey, string(raw))
	})
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	time.Sleep(8 * time.Second)
	return &TunnelEnableOutput{Body: result}, nil
}

// --- Tunnel Disable ---

type TunnelDisableInput struct{}

type TunnelDisableOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

func (s *Handler) HumaTunnelDisable(ctx context.Context, _ *TunnelDisableInput) (*TunnelDisableOutput, error) {
	s.cfManager.Disable(func() {
		current := s.loadAccessSettings(ctx)
		current.TunnelEnabled = false
		raw, _ := json.Marshal(current)
		s.settings.Set(ctx, accessSettingsKey, string(raw))
	})
	return &TunnelDisableOutput{Body: struct {
		Success bool `json:"success"`
	}{Success: true}}, nil
}

// --- Tailscale Check ---

type TailscaleCheckInput struct{}

type TailscaleCheckOutput struct {
	Body any
}

func (s *Handler) HumaTailscaleCheck(ctx context.Context, _ *TailscaleCheckInput) (*TailscaleCheckOutput, error) {
	result := s.tsManager.Check("")
	return &TailscaleCheckOutput{Body: result}, nil
}

// --- Tailscale Enable ---

type TailscaleEnableInput struct {
	Body struct {
		SudoPassword string `json:"sudoPassword,omitempty"`
	}
}

type TailscaleEnableOutput struct {
	Body any
}

func (s *Handler) HumaTailscaleEnable(ctx context.Context, input *TailscaleEnableInput) (*TailscaleEnableOutput, error) {
	result, err := s.tsManager.Enable(input.Body.SudoPassword, func(tunnelURL string) {
		current := s.loadAccessSettings(ctx)
		current.Tailscale = true
		raw, _ := json.Marshal(current)
		s.settings.Set(ctx, accessSettingsKey, string(raw))
	})
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &TailscaleEnableOutput{Body: result}, nil
}

// --- Tailscale Disable ---

type TailscaleDisableInput struct{}

type TailscaleDisableOutput struct {
	Body struct {
		Success bool `json:"success"`
	}
}

func (s *Handler) HumaTailscaleDisable(ctx context.Context, _ *TailscaleDisableInput) (*TailscaleDisableOutput, error) {
	s.tsManager.Disable(func() {
		current := s.loadAccessSettings(ctx)
		current.Tailscale = false
		raw, _ := json.Marshal(current)
		s.settings.Set(ctx, accessSettingsKey, string(raw))
	})
	return &TailscaleDisableOutput{Body: struct {
		Success bool `json:"success"`
	}{Success: true}}, nil
}

// --- List Proxy Pools ---

type ListProxyPoolsInput struct{}

type ListProxyPoolsOutput struct {
	Body struct {
		Pools []map[string]any `json:"pools"`
	}
}

func (s *Handler) HumaListProxyPools(ctx context.Context, _ *ListProxyPoolsInput) (*ListProxyPoolsOutput, error) {
	pools, err := s.pools.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}

	accs, _ := s.accounts.ListByTenant(ctx, adminTenant)
	boundCounts := map[string]int{}
	for _, a := range accs {
		if a.ProxyPoolID != "" {
			boundCounts[a.ProxyPoolID]++
		}
	}

	out := make([]map[string]any, 0, len(pools))
	for _, p := range pools {
		out = append(out, map[string]any{
			"id": p.ID, "name": p.Name, "type": p.Type,
			"proxy_url": p.ProxyURL, "no_proxy": p.NoProxy,
			"strict": p.Strict, "is_active": p.IsActive,
			"test_status": p.TestStatus, "last_tested": p.LastTested,
			"last_error":             p.LastError,
			"bound_connection_count": boundCounts[p.ID],
		})
	}
	return &ListProxyPoolsOutput{Body: struct {
		Pools []map[string]any `json:"pools"`
	}{Pools: out}}, nil
}

// --- Create Proxy Pool ---

type CreateProxyPoolInput struct {
	Body struct {
		Name     string `json:"name"`
		Type     string `json:"type"`
		ProxyURL string `json:"proxy_url"`
		NoProxy  string `json:"no_proxy"`
		Strict   bool   `json:"strict"`
		IsActive *bool  `json:"is_active,omitempty"`
	}
}

type CreateProxyPoolOutput struct {
	Body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
}

func (s *Handler) HumaCreateProxyPool(ctx context.Context, input *CreateProxyPoolInput) (*CreateProxyPoolOutput, error) {
	if input.Body.Name == "" || input.Body.ProxyURL == "" {
		return nil, huma.Error400BadRequest("name and proxy_url are required")
	}

	if err := httputil.ValidateProxyURL(input.Body.ProxyURL); err != nil {
		s.log.Warn("blocked suspicious proxy URL", "url", input.Body.ProxyURL, "error", err)
		return nil, huma.Error400BadRequest("invalid proxy_url: URL blocked by security policy")
	}

	poolType := input.Body.Type
	if poolType == "" {
		poolType = "http"
	}
	if !validProxyPoolTypes[poolType] {
		return nil, huma.Error400BadRequest("invalid pool type: must be http, vercel, cloudflare, or deno")
	}
	active := true
	if input.Body.IsActive != nil {
		active = *input.Body.IsActive
	}
	now := time.Now()
	pool := schema.ProxyPool{
		ID:         uuid.NewString(),
		Name:       input.Body.Name,
		Type:       poolType,
		ProxyURL:   input.Body.ProxyURL,
		NoProxy:    input.Body.NoProxy,
		Strict:     input.Body.Strict,
		IsActive:   active,
		TestStatus: "unknown",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.pools.Create(ctx, pool); err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "proxy pool creation failed"))
	}
	return &CreateProxyPoolOutput{Body: struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}{ID: pool.ID, Name: pool.Name}}, nil
}

// --- Update Proxy Pool ---

type UpdateProxyPoolInput struct {
	ID   string `path:"id" doc:"Proxy pool ID"`
	Body struct {
		Name     *string `json:"name,omitempty"`
		ProxyURL *string `json:"proxy_url,omitempty"`
		NoProxy  *string `json:"no_proxy,omitempty"`
		Strict   *bool   `json:"strict,omitempty"`
		IsActive *bool   `json:"is_active,omitempty"`
	}
}

type UpdateProxyPoolOutput struct{}

func (s *Handler) HumaUpdateProxyPool(ctx context.Context, input *UpdateProxyPoolInput) (*UpdateProxyPoolOutput, error) {
	pool, err := s.pools.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("pool not found")
	}
	if input.Body.Name != nil {
		pool.Name = *input.Body.Name
	}
	if input.Body.ProxyURL != nil {
		if err := httputil.ValidateProxyURL(*input.Body.ProxyURL); err != nil {
			return nil, huma.Error400BadRequest("invalid proxy_url: URL blocked by security policy")
		}
		pool.ProxyURL = *input.Body.ProxyURL
	}
	if input.Body.NoProxy != nil {
		pool.NoProxy = *input.Body.NoProxy
	}
	if input.Body.Strict != nil {
		pool.Strict = *input.Body.Strict
	}
	if input.Body.IsActive != nil {
		pool.IsActive = *input.Body.IsActive
	}
	pool.UpdatedAt = time.Now()
	if err := s.pools.Update(ctx, pool); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	w := openapi.ResponseWriterFromContext(ctx)
	w.WriteHeader(http.StatusNoContent)
	return &UpdateProxyPoolOutput{}, nil
}

// --- Delete Proxy Pool ---

type DeleteProxyPoolInput struct {
	ID string `path:"id" doc:"Proxy pool ID"`
}

type DeleteProxyPoolOutput struct{}

func (s *Handler) HumaDeleteProxyPool(ctx context.Context, input *DeleteProxyPoolInput) (*DeleteProxyPoolOutput, error) {
	accs, _ := s.accounts.ListByTenant(ctx, adminTenant)
	bound := 0
	for _, a := range accs {
		if a.ProxyPoolID == input.ID {
			bound++
		}
	}
	if bound > 0 {
		return nil, huma.Error409Conflict(fmt.Sprintf("proxy pool is currently in use by %d connections", bound))
	}

	if err := s.pools.Delete(ctx, input.ID); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	w := openapi.ResponseWriterFromContext(ctx)
	w.WriteHeader(http.StatusNoContent)
	return &DeleteProxyPoolOutput{}, nil
}

// --- Test Proxy Pool ---

type TestProxyPoolInput struct {
	ID string `path:"id" doc:"Proxy pool ID"`
}

type TestProxyPoolOutput struct {
	Body struct {
		Status     string     `json:"status"`
		LastTested *time.Time `json:"last_tested"`
		ElapsedMS  int64      `json:"elapsed_ms"`
		Error      string     `json:"error"`
	}
}

func (s *Handler) HumaTestProxyPool(ctx context.Context, input *TestProxyPoolInput) (*TestProxyPoolOutput, error) {
	pool, err := s.pools.Get(ctx, input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("pool not found")
	}

	now := time.Now()
	pool.LastTested = &now

	result := testProxyPoolConnectivity(pool)

	pool.TestStatus = result.status
	pool.LastError = result.lastError
	_ = s.pools.Update(ctx, pool)

	return &TestProxyPoolOutput{Body: struct {
		Status     string     `json:"status"`
		LastTested *time.Time `json:"last_tested"`
		ElapsedMS  int64      `json:"elapsed_ms"`
		Error      string     `json:"error"`
	}{
		Status:     pool.TestStatus,
		LastTested: pool.LastTested,
		ElapsedMS:  result.elapsedMS,
		Error:      result.lastError,
	}}, nil
}

// --- List Skills ---

type ListSkillsInput struct{}

type ListSkillsOutput struct {
	Body struct {
		Skills []skill `json:"skills"`
	}
}

func (s *Handler) HumaListSkills(ctx context.Context, _ *ListSkillsInput) (*ListSkillsOutput, error) {
	r := openapi.RequestFromContext(ctx)
	return &ListSkillsOutput{Body: struct {
		Skills []skill `json:"skills"`
	}{Skills: s.loadSkills(r)}}, nil
}

// --- Create Skill ---

type CreateSkillInput struct {
	Body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
		Enabled     *bool  `json:"enabled,omitempty"`
	}
}

type CreateSkillOutput struct {
	Body skill
}

func (s *Handler) HumaCreateSkill(ctx context.Context, input *CreateSkillInput) (*CreateSkillOutput, error) {
	r := openapi.RequestFromContext(ctx)
	if s.settings == nil {
		return nil, huma.Error500InternalServerError("settings store not configured")
	}
	if input.Body.Name == "" {
		return nil, huma.Error400BadRequest("name is required")
	}
	enabled := true
	if input.Body.Enabled != nil {
		enabled = *input.Body.Enabled
	}
	sk := skill{
		ID:          uuid.NewString(),
		Name:        input.Body.Name,
		Description: input.Body.Description,
		Prompt:      input.Body.Prompt,
		Enabled:     enabled,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	skills := append(s.loadSkills(r), sk)
	if err := s.saveSkills(r, skills); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	return &CreateSkillOutput{Body: sk}, nil
}

// --- Update Skill ---

type UpdateSkillInput struct {
	ID   string `path:"id" doc:"Skill ID"`
	Body struct {
		Enabled *bool `json:"enabled,omitempty"`
	}
}

type UpdateSkillOutput struct{}

func (s *Handler) HumaUpdateSkill(ctx context.Context, input *UpdateSkillInput) (*UpdateSkillOutput, error) {
	r := openapi.RequestFromContext(ctx)
	if s.settings == nil {
		return nil, huma.Error500InternalServerError("settings store not configured")
	}
	skills := s.loadSkills(r)
	found := false
	for i := range skills {
		if skills[i].ID == input.ID {
			if input.Body.Enabled != nil {
				skills[i].Enabled = *input.Body.Enabled
			}
			found = true
			break
		}
	}
	if !found {
		return nil, huma.Error404NotFound("skill not found")
	}
	if err := s.saveSkills(r, skills); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	w := openapi.ResponseWriterFromContext(ctx)
	w.WriteHeader(http.StatusNoContent)
	return &UpdateSkillOutput{}, nil
}

// --- Delete Skill ---

type DeleteSkillInput struct {
	ID string `path:"id" doc:"Skill ID"`
}

type DeleteSkillOutput struct{}

func (s *Handler) HumaDeleteSkill(ctx context.Context, input *DeleteSkillInput) (*DeleteSkillOutput, error) {
	r := openapi.RequestFromContext(ctx)
	if s.settings == nil {
		return nil, huma.Error500InternalServerError("settings store not configured")
	}
	skills := s.loadSkills(r)
	out := skills[:0]
	for _, sk := range skills {
		if sk.ID != input.ID {
			out = append(out, sk)
		}
	}
	if err := s.saveSkills(r, out); err != nil {
		return nil, huma.Error500InternalServerError(err.Error())
	}
	w := openapi.ResponseWriterFromContext(ctx)
	w.WriteHeader(http.StatusNoContent)
	return &DeleteSkillOutput{}, nil
}

// --- List Account Health ---

type ListAccountHealthInput struct{}

type ListAccountHealthOutput struct {
	Body struct {
		Health []map[string]any `json:"health"`
	}
}

func (s *Handler) HumaListAccountHealth(ctx context.Context, _ *ListAccountHealthInput) (*ListAccountHealthOutput, error) {
	if s.health == nil {
		return nil, huma.Error500InternalServerError("health repository not configured")
	}
	rows, err := s.health.List(ctx, adminTenant)
	if err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "internal server error"))
	}
	out := make([]map[string]any, 0, len(rows))
	for _, h := range rows {
		out = append(out, accountHealthJSON(h))
	}
	return &ListAccountHealthOutput{Body: struct {
		Health []map[string]any `json:"health"`
	}{Health: out}}, nil
}

// --- Run Health Check ---

type RunHealthCheckInput struct{}

type RunHealthCheckOutput struct {
	Body struct {
		Status string `json:"status"`
	}
}

func (s *Handler) HumaRunHealthCheck(ctx context.Context, _ *RunHealthCheckInput) (*RunHealthCheckOutput, error) {
	r := openapi.RequestFromContext(ctx)
	if s.healthChecker == nil {
		return nil, huma.Error500InternalServerError("health checker not configured")
	}
	checkCtx, cancel := contextWithTimeout(r, s.cfg.Health.Timeout*time.Duration(max(1, s.cfg.Health.MaxParallel)))
	defer cancel()
	s.healthChecker.CheckOnce(checkCtx, adminTenant)
	return &RunHealthCheckOutput{Body: struct {
		Status string `json:"status"`
	}{Status: "ok"}}, nil
}
