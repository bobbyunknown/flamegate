package proxy

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/capability"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// modelEntry is one entry in a /v1/models listing, in the OpenAI shape plus
// FlameGate extensions (provider, kind, dimensions, limits, modalities, pricing).
type modelEntry struct {
	ID               string           `json:"id"`
	Object           string           `json:"object"`
	OwnedBy          string           `json:"owned_by"`
	Provider         string           `json:"provider,omitempty"`
	Kind             string           `json:"kind,omitempty"`
	Name             string           `json:"name,omitempty"`
	Dimensions       int              `json:"dimensions,omitempty"`
	Source           string           `json:"source,omitempty"`
	ContextWindow    int              `json:"context_window,omitempty"`
	MaxOutputTokens  int              `json:"max_output_tokens,omitempty"`
	InputModalities  []string         `json:"input_modalities,omitempty"`
	OutputModalities []string         `json:"output_modalities,omitempty"`
	Pricing          *modelPricingDTO `json:"pricing,omitempty"`
}

type modelPricingDTO struct {
	InputPerM      float64 `json:"input_per_m"`
	OutputPerM     float64 `json:"output_per_m"`
	CachedReadPerM float64 `json:"cached_read_per_m,omitempty"`
	ReasoningPerM  float64 `json:"reasoning_per_m,omitempty"`
}

// listModels is the shared model-listing logic used by both the raw-Chi
// HandleListModels and the Huma HandleListModelsHuma handlers.
func (s *Handler) listModels(ctx context.Context, tenantID string) []modelEntry {
	data := make([]modelEntry, 0, 64)
	seen := make(map[string]struct{}, 64)
	usableProviders := s.usableModelProviders(ctx, tenantID)

	// Chains are exposed as "combo" models, matching the upstream convention:
	// a combo chains multiple providers with auto-fallback and is callable by
	// its bare name (and via the chain: prefix). owned_by:"combo" lets client
	// tools surface them distinctly from single-provider models.
	chains, err := s.chains.ListByTenant(ctx, tenantID)
	if err == nil {
		for _, c := range chains {
			data = s.appendModelEntry(data, seen, modelEntry{
				ID: c.Name, Object: "model", OwnedBy: "combo", Kind: string(core.ServiceLLM), Name: c.Name,
			})
		}
	}

	// Static catalog models for providers the tenant has connected. Without this
	// gate, discovery advertises provider/model ids that the dispatcher will later
	// reject with "no accounts configured".
	for _, pm := range connectors.ModelsByKind(core.ServiceLLM) {
		if !usableProviders[pm.Provider] {
			continue
		}
		prefix := pm.Provider
		if spec, ok := connectors.SpecByAlias(pm.Provider); ok && spec.Alias != "" {
			prefix = spec.Alias
		}
		data = s.appendModelEntry(data, seen, modelEntry{
			ID:       prefix + "/" + pm.Model.ID,
			Object:   "model",
			OwnedBy:  pm.Provider,
			Provider: pm.Provider,
			Kind:     string(core.ServiceLLM),
			Name:     pm.Model.Name,
		})
	}

	// Live model discovery: for providers with a LiveModelSource and connected
	// accounts, fetch the live catalog and merge (live models supplement, not
	// replace, the static catalog).
	liveModels := s.fetchLiveModels(ctx, tenantID)
	for provider, models := range liveModels {
		if !usableProviders[provider] {
			continue
		}
		prefix := provider
		if spec, ok := connectors.SpecByAlias(provider); ok && spec.Alias != "" {
			prefix = spec.Alias
		}
		for _, lm := range models {
			data = s.appendModelEntry(data, seen, modelEntry{
				ID:       prefix + "/" + lm.ID,
				Object:   "model",
				OwnedBy:  provider,
				Provider: provider,
				Kind:     string(lm.Kind),
				Name:     lm.Name,
			})
		}
	}

	// Extension models from WASM extensions (custom > discovered).
	if s.db != nil {
		if extModels, err := s.db.ExtensionModels().ListByTenant(ctx, tenantID); err == nil && len(extModels) > 0 {
			for _, entry := range extensionModelEntries(extModels) {
				data = s.appendModelEntry(data, seen, entry)
			}
		}
	}

	return data
}

// handleListModels reports targetable models: the tenant's chains (as virtual
// models) plus every cataloged LLM model in provider/model form. This lets a
// client discover what it can pass in the `model` field.
func (s *Handler) HandleListModels(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	tenantID := tenantOf(key)
	data := s.listModels(r.Context(), tenantID)
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// handleListModelsByKind serves GET /v1/models/{kind}: it lists every model of
// the requested service kind (llm, embedding, image, stt, tts, search, fetch)
// across the provider catalog, plus a special "chains" view.
func (s *Handler) HandleListModelsByKind(w http.ResponseWriter, r *http.Request) {
	kindParam := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "kind")))

	// "chains" is a convenience view of the tenant's routing chains.
	if kindParam == "chains" {
		s.HandleListModels(w, r)
		return
	}

	kind := core.ServiceKind(kindParam)
	if !core.ValidServiceKind(kind) {
		WriteError(w, http.StatusBadRequest, "unknown model kind: "+kindParam)
		return
	}

	pairs := connectors.ModelsByKind(kind)
	data := make([]modelEntry, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))
	key, _ := authedKey(r.Context())
	usableProviders := s.usableModelProviders(r.Context(), tenantOf(key))
	for _, pm := range pairs {
		if !usableProviders[pm.Provider] {
			continue
		}
		data = s.appendModelEntry(data, seen, modelEntry{
			ID:         pm.Provider + "/" + pm.Model.ID,
			Object:     "model",
			OwnedBy:    pm.Provider,
			Provider:   pm.Provider,
			Kind:       string(pm.Model.Kind),
			Name:       pm.Model.Name,
			Dimensions: pm.Model.Dimensions,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "kind": kindParam, "data": data})
}

// fetchLiveModels queries providers that support live model discovery, using
// the first connected account's credentials. Returns a map of provider →
// models. Errors are silently skipped (live discovery is best-effort).
func (s *Handler) fetchLiveModels(ctx context.Context, tenantID string) map[string][]connectors.ModelSpec {
	if s.accounts == nil || s.vault == nil {
		return nil
	}
	result := map[string][]connectors.ModelSpec{}

	// Check each provider that has a live model source.
	// Live model sources (none currently — providers use WASM list_models)
	for provider, src := range map[string]connectors.LiveModelSource{
	} {
		_, _ = provider, src
		if src == nil {
			continue
		}
		accs, err := s.accounts.ListByProvider(ctx, tenantID, provider)
		if err != nil || len(accs) == 0 {
			continue
		}
		// Use the first non-disabled account.
		var acc schema.Account
		for _, a := range accs {
			if !a.Disabled && !a.NeedsReconnect {
				acc = a
				break
			}
		}
		if acc.ID == "" {
			continue
		}
		creds, err := s.vault.Open(acc)
		if err != nil {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		models, err := src.ListModels(probeCtx, creds)
		cancel()
		if err != nil || len(models) == 0 {
			continue
		}
		result[provider] = models
	}
	return result
}

func (s *Handler) usableModelProviders(ctx context.Context, tenantID string) map[string]bool {
	usable := map[string]bool{}
	if s.accounts == nil {
		return usable
	}
	accs, err := s.accounts.ListByTenant(ctx, tenantID)
	if err != nil {
		return usable
	}
	for _, acc := range accs {
		if acc.Provider == "" || acc.Disabled || acc.NeedsReconnect {
			continue
		}
		usable[acc.Provider] = true
	}
	return usable
}

func (s *Handler) enrichModelEntry(entry *modelEntry) {
	if entry.Kind != string(core.ServiceLLM) && entry.Kind != "" {
		return
	}
	provider := entry.Provider
	modelID := entry.ID
	if provider != "" && strings.HasPrefix(modelID, provider+"/") {
		modelID = strings.TrimPrefix(modelID, provider+"/")
	}

	prof := capability.ResolveProfile(provider, modelID)
	if prof.ContextWindow > 0 {
		entry.ContextWindow = prof.ContextWindow
	}
	if prof.MaxOutput > 0 {
		entry.MaxOutputTokens = prof.MaxOutput
	}

	inputMods := []string{"text"}
	if prof.Vision {
		inputMods = append(inputMods, "image")
	}
	if prof.PDF {
		inputMods = append(inputMods, "pdf")
	}
	if prof.AudioInput {
		inputMods = append(inputMods, "audio")
	}
	if prof.VideoInput {
		inputMods = append(inputMods, "video")
	}
	entry.InputModalities = inputMods

	outputMods := []string{"text"}
	if prof.ImageOutput {
		outputMods = append(outputMods, "image")
	}
	if prof.AudioOutput {
		outputMods = append(outputMods, "audio")
	}
	entry.OutputModalities = outputMods

	if s.catalog != nil {
		if p, ok := s.catalog.GetPrice(provider, modelID); ok {
			entry.Pricing = &modelPricingDTO{
				InputPerM:      p.InputPerM,
				OutputPerM:     p.OutputPerM,
				CachedReadPerM: p.CachedInputPerM,
				ReasoningPerM:  p.ReasoningPerM,
			}
		}
	}
}

func (s *Handler) appendModelEntry(data []modelEntry, seen map[string]struct{}, entry modelEntry) []modelEntry {
	if entry.ID == "" {
		return data
	}
	if _, ok := seen[entry.ID]; ok {
		return data
	}
	seen[entry.ID] = struct{}{}
	s.enrichModelEntry(&entry)
	return append(data, entry)
}

// extensionModelEntries converts extension DB models into modelEntry structs.
// Custom entries are sorted before discovered so the caller's seen-map dedup
// gives custom priority (custom > discovered).
func extensionModelEntries(models []schema.ExtensionModel) []modelEntry {
	if len(models) == 0 {
		return nil
	}
	// Sort: custom first, then discovered. Stable within same source.
	entries := make([]modelEntry, 0, len(models))
	for _, m := range models {
		if m.Source == "custom" {
			entries = append(entries, extensionModelToEntry(m))
		}
	}
	for _, m := range models {
		if m.Source != "custom" {
			entries = append(entries, extensionModelToEntry(m))
		}
	}
	return entries
}

// extensionModelToEntry converts a single ExtensionModel DB row to a modelEntry.
// Rows store model id in Slug and provider/model in ID (e.g. xiaomi-mimo/mimo-v2.5).
func extensionModelToEntry(m schema.ExtensionModel) modelEntry {
	provider, modelID := splitExtensionModelID(m.ID)
	if modelID == "" {
		modelID = strings.TrimSpace(m.Slug)
	}
	if provider == "" {
		// Legacy rows used Slug as provider id — do not invent a model path.
		provider = strings.TrimSpace(m.Slug)
		if modelID == provider {
			modelID = ""
		}
	}
	if modelID == "" {
		modelID = strings.TrimSpace(m.Slug)
	}
	prefix := provider
	if spec, ok := connectors.SpecByAlias(provider); ok && spec.Alias != "" {
		prefix = spec.Alias
	}
	id := modelID
	if prefix != "" && modelID != "" {
		id = prefix + "/" + modelID
	} else if prefix != "" {
		id = prefix
	}
	name := strings.TrimSpace(m.DisplayName)
	if name == "" {
		name = modelID
	}
	if name == "" {
		name = id
	}
	return modelEntry{
		ID:       id,
		Object:   "model",
		OwnedBy:  provider,
		Provider: provider,
		Kind:     string(core.ServiceLLM),
		Name:     name,
		Source:   m.Source,
	}
}

func splitExtensionModelID(id string) (provider, model string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", ""
	}
	provider, model, ok := strings.Cut(id, "/")
	if !ok {
		return "", id
	}
	return provider, model
}

// HandleModelInfo serves GET /v1/models/info?id=<provider/model>: it returns
// metadata for a single model (kind, dimensions, provider, name, limits, modalities, pricing).
func (s *Handler) HandleModelInfo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		WriteError(w, http.StatusBadRequest, "id query parameter is required")
		return
	}

	provider, model, ok := strings.Cut(id, "/")
	if !ok || provider == "" || model == "" {
		WriteError(w, http.StatusBadRequest, "id must be in provider/model form")
		return
	}
	if spec, ok := connectors.SpecByAlias(provider); ok {
		provider = spec.ID
	}
	key, _ := authedKey(r.Context())
	if !s.usableModelProviders(r.Context(), tenantOf(key))[provider] {
		WriteError(w, http.StatusNotFound, "unknown model: "+id)
		return
	}

	// 1. Static connector registry
	if spec, found := connectors.FindModel(provider, model); found {
		entry := modelEntry{
			ID:         id,
			Object:     "model",
			OwnedBy:    provider,
			Provider:   provider,
			Kind:       string(spec.Kind),
			Name:       spec.Name,
			Dimensions: spec.Dimensions,
		}
		s.enrichModelEntry(&entry)
		writeJSON(w, http.StatusOK, map[string]any{
			"id":                entry.ID,
			"provider":          entry.Provider,
			"model":             spec.ID,
			"name":              entry.Name,
			"kind":              entry.Kind,
			"dimensions":        entry.Dimensions,
			"context_window":    entry.ContextWindow,
			"max_output_tokens": entry.MaxOutputTokens,
			"input_modalities":  entry.InputModalities,
			"output_modalities": entry.OutputModalities,
			"pricing":           entry.Pricing,
		})
		return
	}

	// 2. ExtensionModels from DB
	if s.db != nil {
		if extModel, err := s.db.ExtensionModels().Get(r.Context(), id); err == nil && extModel.ID != "" {
			entry := extensionModelToEntry(extModel)
			s.enrichModelEntry(&entry)
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                entry.ID,
				"provider":          entry.Provider,
				"model":             model,
				"name":              entry.Name,
				"kind":              entry.Kind,
				"dimensions":        entry.Dimensions,
				"context_window":    entry.ContextWindow,
				"max_output_tokens": entry.MaxOutputTokens,
				"input_modalities":  entry.InputModalities,
				"output_modalities": entry.OutputModalities,
				"pricing":           entry.Pricing,
				"source":            entry.Source,
			})
			return
		}
	}

	// 3. Dynamic Catalog Service
	if s.catalog != nil {
		if catModel, ok := s.catalog.FindModel(provider, model); ok && catModel != nil {
			entry := modelEntry{
				ID:       id,
				Object:   "model",
				OwnedBy:  provider,
				Provider: provider,
				Kind:     string(core.ServiceLLM),
				Name:     catModel.Name,
			}
			s.enrichModelEntry(&entry)
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                entry.ID,
				"provider":          entry.Provider,
				"model":             model,
				"name":              entry.Name,
				"kind":              entry.Kind,
				"dimensions":        entry.Dimensions,
				"context_window":    entry.ContextWindow,
				"max_output_tokens": entry.MaxOutputTokens,
				"input_modalities":  entry.InputModalities,
				"output_modalities": entry.OutputModalities,
				"pricing":           entry.Pricing,
			})
			return
		}
	}

	WriteError(w, http.StatusNotFound, "unknown model: "+id)
}
