package admin

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/httputil"
)

// mountCustomProviders registers the dynamic custom-provider and custom-model
// admin endpoints. Custom providers are user-defined instances of the OpenAI-
// or Anthropic-compatible dialects, each isolated under a unique provider id.
func (s *Handler) MountCustomProviders(r chi.Router) {
	r.Get("/custom-providers", s.adminListCustomProviders)
	r.Post("/custom-providers", s.adminCreateCustomProvider)
	r.Patch("/custom-providers/{id}", s.adminUpdateCustomProvider)
	r.Delete("/custom-providers/{id}", s.adminDeleteCustomProvider)

	// Custom models can be attached to any provider id (custom or built-in).
	r.Get("/providers/{id}/custom-models", s.adminListCustomModels)
	r.Post("/providers/{id}/custom-models", s.adminCreateCustomModel)
	r.Patch("/providers/{id}/custom-models/{modelDBID}", s.adminUpdateCustomModel)
	r.Delete("/providers/{id}/custom-models/{modelDBID}", s.adminDeleteCustomModel)
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify produces a short url-safe token from a display name.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 32 {
		s = s[:32]
		s = strings.Trim(s, "-")
	}
	return s
}

func customProviderJSON(p schema.CustomProvider) map[string]any {
	return map[string]any{
		"id":           p.ID,
		"display_name": p.DisplayName,
		"alias":        p.Alias,
		"dialect":      p.Dialect,
		"base_url":     p.BaseURL,
		"custom":       true,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
	}
}

func customModelJSON(m schema.CustomModel) map[string]any {
	return map[string]any{
		"db_id":          m.ID,
		"provider_id":    m.ProviderID,
		"id":             m.ModelID,
		"name":           m.DisplayName,
		"kind":           m.Kind,
		"context_window": m.ContextWindow,
		"input_per_m":    m.InputPerM,
		"output_per_m":   m.OutputPerM,
	}
}

func (s *Handler) adminListCustomProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := s.db.CustomProviders().ListProviders(r.Context(), adminTenant)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(providers))
	for _, p := range providers {
		out = append(out, customProviderJSON(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": out})
}

func (s *Handler) adminCreateCustomProvider(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName string `json:"display_name"`
		Dialect     string `json:"dialect"` // "openai" | "anthropic"
		BaseURL     string `json:"base_url"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	name := strings.TrimSpace(body.DisplayName)
	if name == "" {
		WriteError(w, http.StatusBadRequest, "display_name is required")
		return
	}
	dialect, prefix, ok := customDialect(body.Dialect)
	if !ok {
		WriteError(w, http.StatusBadRequest, "dialect must be 'openai' or 'anthropic'")
		return
	}
	baseURL := strings.TrimSpace(body.BaseURL)
	if baseURL == "" {
		WriteError(w, http.StatusBadRequest, "base_url is required")
		return
	}
	if err := httputil.ValidateBaseURL(baseURL); err != nil {
		s.log.Warn("blocked suspicious base_url", "url", baseURL, "error", err)
		WriteError(w, http.StatusBadRequest, "invalid base_url: URL blocked by security policy")
		return
	}

	id := uniqueCustomProviderID(prefix, name, func(candidate string) bool {
		if _, exists := connectors.SpecByID(candidate); exists {
			return true
		}
		return false
	})

	p := schema.CustomProvider{
		ID:          id,
		TenantID:    adminTenant,
		DisplayName: name,
		Alias:       id, // alias defaults to the full unique id; user can override
		Dialect:     string(dialect),
		BaseURL:     baseURL,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := s.db.CustomProviders().CreateProvider(r.Context(), p); err != nil {
		WriteError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "create custom provider failed"))
		return
	}
	// Register it live so it is immediately routable and discoverable.
	connectors.RegisterDynamicProvider(connectors.DynamicProvider{
		ID: p.ID, DisplayName: p.DisplayName, Alias: p.Alias, Dialect: dialect, BaseURL: p.BaseURL,
	})
	writeJSON(w, http.StatusCreated, customProviderJSON(p))
}

func (s *Handler) adminUpdateCustomProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.db.CustomProviders().GetProvider(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "unknown custom provider: "+id)
		return
	}
	var body struct {
		DisplayName *string `json:"display_name"`
		Alias       *string `json:"alias"`
		BaseURL     *string `json:"base_url"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.DisplayName != nil {
		if n := strings.TrimSpace(*body.DisplayName); n != "" {
			existing.DisplayName = n
		}
	}
	if body.Alias != nil {
		existing.Alias = strings.TrimSpace(*body.Alias)
		if existing.Alias == "" {
			existing.Alias = existing.ID
		}
	}
	if body.BaseURL != nil {
		b := strings.TrimSpace(*body.BaseURL)
		if b == "" {
			WriteError(w, http.StatusBadRequest, "base_url cannot be empty")
			return
		}
		if err := httputil.ValidateBaseURL(b); err != nil {
			s.log.Warn("blocked suspicious base_url", "url", b, "error", err)
			WriteError(w, http.StatusBadRequest, "invalid base_url: URL blocked by security policy")
			return
		}
		existing.BaseURL = b
	}
	if err := s.db.CustomProviders().UpdateProvider(r.Context(), existing); err != nil {
		WriteError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "update custom provider failed"))
		return
	}
	dialect, _, _ := customDialect(existing.Dialect)
	connectors.RegisterDynamicProvider(connectors.DynamicProvider{
		ID: existing.ID, DisplayName: existing.DisplayName, Alias: existing.Alias, Dialect: dialect, BaseURL: existing.BaseURL,
	})
	writeJSON(w, http.StatusOK, customProviderJSON(existing))
}

func (s *Handler) adminDeleteCustomProvider(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.db.CustomProviders().GetProvider(r.Context(), id); err != nil {
		WriteError(w, http.StatusNotFound, "unknown custom provider: "+id)
		return
	}
	if err := s.db.CustomProviders().DeleteProvider(r.Context(), id); err != nil {
		WriteError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "delete custom provider failed"))
		return
	}
	connectors.UnregisterDynamicProvider(id)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// ---- custom models ----------------------------------------------------------

func (s *Handler) adminListCustomModels(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "id")
	models, err := s.db.CustomProviders().ListModelsByProvider(r.Context(), providerID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "internal server error"))
		return
	}
	out := make([]map[string]any, 0, len(models))
	for _, m := range models {
		out = append(out, customModelJSON(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": out})
}

func (s *Handler) adminCreateCustomModel(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "id")
	if _, ok := connectors.SpecByID(providerID); !ok {
		WriteError(w, http.StatusNotFound, "unknown provider: "+providerID)
		return
	}
	var body struct {
		ModelID       string  `json:"id"`
		DisplayName   string  `json:"name"`
		Kind          string  `json:"kind"`
		ContextWindow int     `json:"context_window"`
		InputPerM     float64 `json:"input_per_m"`
		OutputPerM    float64 `json:"output_per_m"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	modelID := strings.TrimSpace(body.ModelID)
	if modelID == "" {
		WriteError(w, http.StatusBadRequest, "model id is required")
		return
	}
	kind := normalizeModelKind(body.Kind)
	if !core.ValidServiceKind(core.ServiceKind(kind)) {
		WriteError(w, http.StatusBadRequest, "unknown model kind: "+body.Kind)
		return
	}
	name := strings.TrimSpace(body.DisplayName)
	if name == "" {
		name = modelID
	}
	m := schema.CustomModel{
		ID:            uuid.NewString(),
		TenantID:      adminTenant,
		ProviderID:    providerID,
		ModelID:       modelID,
		DisplayName:   name,
		Kind:          kind,
		ContextWindow: body.ContextWindow,
		InputPerM:     body.InputPerM,
		OutputPerM:    body.OutputPerM,
	}
	if err := s.db.CustomProviders().CreateModel(r.Context(), m); err != nil {
		WriteError(w, http.StatusBadRequest, sanitizeError(s.log, err, "create custom model failed (model id may already exist)"))
		return
	}
	s.reloadCustomModels(r.Context(), providerID)
	writeJSON(w, http.StatusCreated, customModelJSON(m))
}

func (s *Handler) adminUpdateCustomModel(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "id")
	dbID := chi.URLParam(r, "modelDBID")
	existing, err := s.db.CustomProviders().GetModel(r.Context(), dbID)
	if err != nil || existing.ProviderID != providerID {
		WriteError(w, http.StatusNotFound, "unknown custom model")
		return
	}
	var body struct {
		ModelID       *string  `json:"id"`
		DisplayName   *string  `json:"name"`
		Kind          *string  `json:"kind"`
		ContextWindow *int     `json:"context_window"`
		InputPerM     *float64 `json:"input_per_m"`
		OutputPerM    *float64 `json:"output_per_m"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ModelID != nil {
		if v := strings.TrimSpace(*body.ModelID); v != "" {
			existing.ModelID = v
		}
	}
	if body.DisplayName != nil {
		existing.DisplayName = strings.TrimSpace(*body.DisplayName)
	}
	if existing.DisplayName == "" {
		existing.DisplayName = existing.ModelID
	}
	if body.Kind != nil {
		kind := normalizeModelKind(*body.Kind)
		if !core.ValidServiceKind(core.ServiceKind(kind)) {
			WriteError(w, http.StatusBadRequest, "unknown model kind: "+*body.Kind)
			return
		}
		existing.Kind = kind
	}
	if body.ContextWindow != nil {
		existing.ContextWindow = *body.ContextWindow
	}
	if body.InputPerM != nil {
		existing.InputPerM = *body.InputPerM
	}
	if body.OutputPerM != nil {
		existing.OutputPerM = *body.OutputPerM
	}
	if err := s.db.CustomProviders().UpdateModel(r.Context(), existing); err != nil {
		WriteError(w, http.StatusBadRequest, sanitizeError(s.log, err, "update custom model failed"))
		return
	}
	s.reloadCustomModels(r.Context(), providerID)
	writeJSON(w, http.StatusOK, customModelJSON(existing))
}

func (s *Handler) adminDeleteCustomModel(w http.ResponseWriter, r *http.Request) {
	providerID := chi.URLParam(r, "id")
	dbID := chi.URLParam(r, "modelDBID")
	existing, err := s.db.CustomProviders().GetModel(r.Context(), dbID)
	if err != nil || existing.ProviderID != providerID {
		WriteError(w, http.StatusNotFound, "unknown custom model")
		return
	}
	if err := s.db.CustomProviders().DeleteModel(r.Context(), dbID); err != nil {
		WriteError(w, http.StatusInternalServerError, sanitizeError(s.log, err, "delete custom model failed"))
		return
	}
	s.reloadCustomModels(r.Context(), providerID)
	writeJSON(w, http.StatusOK, map[string]any{"db_id": dbID, "deleted": true})
}

// reloadCustomModels refreshes the in-memory dynamic model set for a provider
// from the database so routing/discovery reflect the change immediately.
func (s *Handler) reloadCustomModels(ctx context.Context, providerID string) {
	models, err := s.db.CustomProviders().ListModelsByProvider(ctx, providerID)
	if err != nil {
		s.log.Warn("reload custom models failed", "provider", providerID, "err", err)
		return
	}
	connectors.SetDynamicModels(providerID, customModelsToSpecs(models))
}

func customModelsToSpecs(models []schema.CustomModel) []connectors.ModelSpec {
	out := make([]connectors.ModelSpec, 0, len(models))
	for _, m := range models {
		kind := core.ServiceKind(m.Kind)
		if kind == "" {
			kind = core.ServiceLLM
		}
		out = append(out, connectors.ModelSpec{ID: m.ModelID, Name: m.DisplayName, Kind: kind})
	}
	return out
}

func normalizeModelKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return string(core.ServiceLLM)
	}
	return kind
}

// customDialect maps a user-supplied dialect token to a core dialect and the
// provider-id prefix for that dialect.
func customDialect(token string) (core.Dialect, string, bool) {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "openai", "openai-compatible", "":
		return core.DialectOpenAI, connectors.CustomOpenAIPrefix, true
	case "anthropic", "anthropic-compatible":
		return core.DialectAnthropic, connectors.CustomAnthropicPrefix, true
	default:
		return "", "", false
	}
}

// uniqueCustomProviderID builds a unique provider id from a prefix and a name
// slug, appending a short uuid suffix when the slug collides.
func uniqueCustomProviderID(prefix, name string, taken func(string) bool) string {
	slug := slugify(name)
	if slug == "" {
		slug = "instance"
	}
	candidate := prefix + slug
	if !taken(candidate) {
		return candidate
	}
	return prefix + slug + "-" + strings.Split(uuid.NewString(), "-")[0]
}
