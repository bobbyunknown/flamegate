package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
)

// MountExtensions registers the extension management admin endpoints.
func (s *Handler) MountExtensions(r chi.Router) {
	r.Post("/extensions/install", s.adminInstallExtension)
	r.Get("/extensions", s.adminListExtensions)
	r.Get("/extensions/{slug}", s.adminGetExtension)
	r.Delete("/extensions/{slug}", s.adminUninstallExtension)
	r.Post("/extensions/{slug}/enable", s.adminEnableExtension)
	r.Post("/extensions/{slug}/disable", s.adminDisableExtension)
	r.Post("/extensions/{slug}/sync-models", s.adminSyncExtensionModels)
	// Per-extension toggle: whether install/enable auto-runs list_models into DB.
	r.Put("/extensions/{slug}/auto-sync-models", s.adminSetAutoSyncModels)
}

// adminInstallExtension handles multipart install: .wasm + schema.json.
func (s *Handler) adminInstallExtension(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseMultipartForm(64 << 20); err != nil { // 64MB max
		WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	// Read schema.json.
	schemaFile, _, err := r.FormFile("schema")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "missing 'schema' file field")
		return
	}
	defer schemaFile.Close()

	schemaData, err := io.ReadAll(schemaFile)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "failed to read schema file")
		return
	}

	var extSchema wasm.ExtensionSchema
	if err := json.Unmarshal(schemaData, &extSchema); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid schema.json: "+err.Error())
		return
	}

	slug := extSchema.Slug
	if slug == "" {
		WriteError(w, http.StatusBadRequest, "schema.json missing 'slug'")
		return
	}

	// Validate slug format.
	if !wasm.IsValidSlug(slug) {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid slug format: %q", slug))
		return
	}

	// Reject native provider slugs.
	if connectors.IsNativeSlug(slug) {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("slug %q is a native provider", slug))
		return
	}

	// Check not already installed.
	if _, err := s.db.Extensions().FindBySlug(ctx, slug); err == nil {
		WriteError(w, http.StatusConflict, fmt.Sprintf("extension %q already installed", slug))
		return
	}

	// Read .wasm file.
	wasmFile, _, err := r.FormFile("wasm")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "missing 'wasm' file field")
		return
	}
	defer wasmFile.Close()

	wasmBytes, err := io.ReadAll(wasmFile)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "failed to read wasm file")
		return
	}

	if len(wasmBytes) == 0 {
		WriteError(w, http.StatusBadRequest, "wasm file is empty")
		return
	}

	// Write files to extension directory.
	extDir := s.cfg.WASM.ExtDir
	if extDir == "" {
		WriteError(w, http.StatusInternalServerError, "extension directory not configured")
		return
	}

	slugDir := filepath.Join(extDir, slug)
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to create extension directory")
		return
	}

	wasmPath := filepath.Join(slugDir, slug+".wasm")
	if err := os.WriteFile(wasmPath, wasmBytes, 0o644); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write wasm file")
		return
	}

	schemaPath := filepath.Join(slugDir, "schema.json")
	if err := os.WriteFile(schemaPath, schemaData, 0o644); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write schema file")
		return
	}

	// Compile extension.
	extCfg := wasm.ExtensionConfig{
		Slug:         slug,
		Timeout:      time.Duration(extSchema.Timeout) * time.Second,
		AllowedHosts: s.cfg.WASM.AllowedHosts,
		Entrypoints:  extSchema.Entrypoints,
	}

	if s.wasmEngine == nil {
		WriteError(w, http.StatusInternalServerError, "WASM engine not available")
		return
	}

	if err := s.wasmEngine.Compile(ctx, slug, wasmBytes, extCfg); err != nil {
		// Clean up files on compile failure.
		_ = os.RemoveAll(slugDir)
		WriteError(w, http.StatusBadRequest, "compile failed: "+err.Error())
		return
	}

	// Create DB record.
	ext := schema.Extension{
		ID:                uuid.NewString(),
		TenantID:          adminTenant,
		Slug:              slug,
		Name:              extSchema.Name,
		Version:           extSchema.Version,
		Description:       extSchema.Description,
		WasmPath:          wasmPath,
		SchemaPath:        schemaPath,
		State:             "ACTIVE",
		Capabilities:      mustJSON(extSchema.Entrypoints),
		Entrypoints:       mustJSON(extSchema.Entrypoints),
		Config:            mustJSON(map[string]any{"timeout": extSchema.Timeout, "max_instances": extSchema.MaxInstances}),
		DefaultAccountKey: extSchema.DefaultAccountKey,
		AuthKind:          firstAuthMode(extSchema.AuthModes),
		AutoSyncModels:    true, // default on; override via form field auto_sync_models=false
		CompiledAt:        new(time.Now()),
		InstalledAt:       time.Now(),
		UpdatedAt:         time.Now(),
	}
	// Optional multipart form field: auto_sync_models=true|false (default true).
	if v := strings.TrimSpace(r.FormValue("auto_sync_models")); v != "" {
		ext.AutoSyncModels = parseBoolDefault(v, true)
	}

	if err := s.db.Extensions().Create(ctx, ext); err != nil {
		_ = s.wasmEngine.Unload(slug)
		_ = os.RemoveAll(slugDir)
		WriteError(w, http.StatusInternalServerError, "failed to save extension: "+err.Error())
		return
	}

	// Register module for lifecycle operations.
	if s.wasmModules != nil {
		s.wasmModules[slug] = wasm.NewModule(s.wasmEngine, slug, s.cfg.WASM.MaxInst)
	}
	connectors.RegisterExtensionSpec(connectors.ProviderSpec{
		ID:           slug,
		DisplayName:  extSchema.Name,
		Alias:        slug,
		Dialect:      core.DialectOpenAI,
		AuthKind:     ext.AuthKind,
		AuthModes:    extSchema.AuthModes,
		ServiceKinds: []core.ServiceKind{core.ServiceLLM},
		Notice:       "WASM extension",
	})

	// Auto-discover models only when per-extension flag is on.
	synced := 0
	if ext.AutoSyncModels {
		n, syncErr := s.syncExtensionModels(ctx, ext)
		if syncErr != nil {
			s.log.WithError(syncErr).Warn("install: auto sync models failed", "slug", slug)
		} else {
			synced = n
		}
	}

	writeJSON(w, http.StatusCreated, extensionJSON(ext, synced))
}
// firstAuthMode returns the first supported auth mode (oauth preferred), or
// api_key when the extension declares none.
func firstAuthMode(modes []string) string {
	for _, m := range modes {
		switch m {
		case "oauth", "api_key", "none":
			return m
		}
	}
	return "api_key"
}

// adminListExtensions returns all installed extensions.
func (s *Handler) adminListExtensions(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		WriteError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	ctx := r.Context()
	exts, err := s.db.Extensions().List(ctx, adminTenant)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to list extensions")
		return
	}

	out := make([]map[string]any, 0, len(exts))
	for _, ext := range exts {
		modelCount, _ := s.countModels(ctx, ext.ID)
		out = append(out, extensionJSON(ext, modelCount))
	}

	writeJSON(w, http.StatusOK, map[string]any{"extensions": out})
}

// adminGetExtension returns details for a single extension.
func (s *Handler) adminGetExtension(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	ext, err := s.db.Extensions().FindBySlug(ctx, slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("extension %q not found", slug))
		return
	}

	modelCount, _ := s.countModels(ctx, ext.ID)
	writeJSON(w, http.StatusOK, extensionJSON(ext, modelCount))
}

// adminUninstallExtension removes an extension: unload from engine, delete DB + models.
func (s *Handler) adminUninstallExtension(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		WriteError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	ext, err := s.db.Extensions().FindBySlug(ctx, slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("extension %q not found", slug))
		return
	}

	// Unload from WASM engine (ignore if not compiled).
	if s.wasmEngine != nil {
		_ = s.wasmEngine.Unload(slug)
	}

	// Remove from module map.
	if s.wasmModules != nil {
		delete(s.wasmModules, slug)
	}
	connectors.UnregisterExtensionSpec(slug)


	// Delete all extension models.
	if err := s.db.ExtensionModels().DeleteByExtension(ctx, ext.ID); err != nil {
		s.log.WithError(err).Warn("uninstall: failed to delete models", "slug", slug)
	}

	// Delete extension DB record.
	if err := s.db.Extensions().Delete(ctx, ext.ID); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to delete extension")
		return
	}

	// Remove files from disk.
	if ext.WasmPath != "" {
		dir := filepath.Dir(ext.WasmPath)
		if dir != filepath.Clean(s.cfg.WASM.ExtDir) {
			_ = os.RemoveAll(dir)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slug": slug})
}

// adminEnableExtension transitions a DISABLED extension back to ACTIVE.
func (s *Handler) adminEnableExtension(w http.ResponseWriter, r *http.Request) {
	s.setExtensionState(w, r, wasm.StateActive, "ACTIVE")
}

// adminDisableExtension transitions an ACTIVE extension to DISABLED.
func (s *Handler) adminDisableExtension(w http.ResponseWriter, r *http.Request) {
	s.setExtensionState(w, r, wasm.StateDisabled, "DISABLED")
}

// setExtensionState handles enable/disable by transitioning module state.
func (s *Handler) setExtensionState(w http.ResponseWriter, r *http.Request, state int32, stateStr string) {
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")

	ext, err := s.db.Extensions().FindBySlug(ctx, slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("extension %q not found", slug))
		return
	}

	// Idempotent: if already in target state, return success.
	if ext.State == stateStr {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slug": slug, "state": stateStr})
		return
	}

	// Get or create module.
	mod := s.getOrCreateModule(slug)
	if mod == nil {
		WriteError(w, http.StatusInternalServerError, "WASM engine not available")
		return
	}

	log := s.log.WithField("slug", slug)
	if err := wasm.Transition(mod, state, log); err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid transition from %s to %s: %s", ext.State, stateStr, err))
		return
	}

	// Update DB.
	ext.State = stateStr
	ext.UpdatedAt = time.Now()
	if err := s.db.Extensions().Update(ctx, ext); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to update extension state")
		return
	}
	if stateStr == "ACTIVE" {
		connectors.RegisterExtensionSpec(connectors.ProviderSpec{
			ID:           slug,
			DisplayName:  ext.Name,
			Alias:        slug,
			Dialect:      core.DialectOpenAI,
			AuthKind:     ext.AuthKind,
			ServiceKinds: []core.ServiceKind{core.ServiceLLM},
			Notice:       "WASM extension",
		})
		// Only auto-sync when this extension opted in.
		if ext.AutoSyncModels {
			if _, err := s.syncExtensionModels(ctx, ext); err != nil {
				s.log.WithError(err).Warn("enable: auto sync models failed", "slug", slug)
			}
		}
	} else {
		connectors.UnregisterExtensionSpec(slug)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "slug": slug, "state": stateStr})
}

// adminSyncExtensionModels calls the extension's list_models entrypoint,
// deletes all discovered models for the extension, and inserts fresh ones.
func (s *Handler) adminSyncExtensionModels(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		WriteError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		WriteError(w, http.StatusBadRequest, "missing slug")
		return
	}

	if connectors.IsNativeSlug(slug) {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("slug %q is a native provider, not an extension", slug))
		return
	}

	ext, err := s.db.Extensions().FindBySlug(ctx, slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("extension %q not found", slug))
		return
	}

	if s.wasmEngine == nil {
		WriteError(w, http.StatusServiceUnavailable, "WASM engine not available")
		return
	}

	inserted, err := s.syncExtensionModels(ctx, ext)
	if err != nil {
		s.log.WithError(err).Warn("extension sync: list_models failed", "slug", slug)
		WriteError(w, http.StatusBadGateway, fmt.Sprintf("list_models failed: %s", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"slug":   slug,
		"synced": inserted,
	})
}

// adminSetAutoSyncModels sets whether install/enable auto-runs model discovery.
// Body: {"auto_sync_models": true|false}
// Manual POST .../sync-models always works regardless of this flag.
func (s *Handler) adminSetAutoSyncModels(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		WriteError(w, http.StatusServiceUnavailable, "database not available")
		return
	}
	ctx := r.Context()
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		WriteError(w, http.StatusBadRequest, "missing slug")
		return
	}
	ext, err := s.db.Extensions().FindBySlug(ctx, slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("extension %q not found", slug))
		return
	}
	var body struct {
		AutoSyncModels *bool `json:"auto_sync_models"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.AutoSyncModels == nil {
		WriteError(w, http.StatusBadRequest, "auto_sync_models is required (true or false)")
		return
	}
	ext.AutoSyncModels = *body.AutoSyncModels
	ext.UpdatedAt = time.Now()
	if err := s.db.Extensions().Update(ctx, ext); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to update auto_sync_models")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"slug":             slug,
		"auto_sync_models": ext.AutoSyncModels,
	})
}

// syncExtensionModels invokes WASM list_models and replaces discovered rows.
// Returns inserted count. Custom-source rows are preserved.
func (s *Handler) syncExtensionModels(ctx context.Context, ext schema.Extension) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not available")
	}
	if s.wasmEngine == nil {
		return 0, fmt.Errorf("WASM engine not available")
	}
	slug := ext.Slug
	creds := s.extensionCreds(ctx, ext)
	models, err := s.wasmEngine.ListModels(ctx, slug, creds)
	if err != nil {
		return 0, err
	}
	if err := s.db.ExtensionModels().DeleteBySource(ctx, ext.ID, "discovered"); err != nil {
		return 0, fmt.Errorf("clear discovered: %w", err)
	}
	inserted := 0
	for _, m := range models {
		modelID := strings.TrimSpace(m.ID)
		if modelID == "" {
			continue
		}
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = modelID
		}
		var metadataJSON string
		metaMap := map[string]any{}
		if m.Tier != "" {
			metaMap["tier"] = m.Tier
		}
		if m.Category != "" {
			metaMap["category"] = m.Category
		}
		if len(m.Tags) > 0 {
			metaMap["tags"] = m.Tags
		}
		if len(metaMap) > 0 {
			if b, err := json.Marshal(metaMap); err == nil {
				metadataJSON = string(b)
			}
		}

		em := schema.ExtensionModel{
			ID:          fmt.Sprintf("%s/%s", slug, modelID),
			ExtensionID: ext.ID,
			TenantID:    ext.TenantID,
			Slug:        modelID,
			DisplayName: name,
			Source:      "discovered",
			Enabled:     true,
			Metadata:    metadataJSON,
		}
		if err := s.db.ExtensionModels().Create(ctx, em); err != nil {
			s.log.WithError(err).Warn("extension sync: insert model failed", "slug", slug, "model", modelID)
			continue
		}
		inserted++
	}
	return inserted, nil
}

// --- helpers ---

func (s *Handler) extensionCreds(ctx context.Context, ext schema.Extension) core.Credentials {
	if s.vault == nil || s.accounts == nil {
		return core.Credentials{}
	}

	accs, err := s.accounts.ListByProvider(ctx, ext.TenantID, ext.Slug)
	if err != nil || len(accs) == 0 {
		return core.Credentials{}
	}

	// Prefer default_account_key label when set; otherwise first usable account.
	// (schema default_account_key is often "default" while UI labels are emails.)
	var fallback core.Credentials
	haveFallback := false
	for _, acc := range accs {
		if acc.Disabled || acc.NeedsReconnect {
			continue
		}
		creds, err := s.vault.Open(acc)
		if err != nil {
			continue
		}
		if ext.DefaultAccountKey != "" && acc.Label == ext.DefaultAccountKey {
			return creds
		}
		if !haveFallback {
			fallback = creds
			haveFallback = true
		}
	}
	if haveFallback {
		return fallback
	}
	return core.Credentials{}
}

func (s *Handler) countModels(ctx context.Context, extID string) (int, error) {
	models, err := s.db.ExtensionModels().ListByExtension(ctx, extID)
	if err != nil {
		return 0, err
	}
	return len(models), nil
}

func (s *Handler) getOrCreateModule(slug string) *wasm.Module {
	if s.wasmModules == nil {
		return nil
	}
	mod, ok := s.wasmModules[slug]
	if !ok {
		if s.wasmEngine == nil {
			return nil
		}
		mod = wasm.NewModule(s.wasmEngine, slug, s.cfg.WASM.MaxInst)
		s.wasmModules[slug] = mod
	}
	return mod
}

func extensionJSON(ext schema.Extension, modelCount int) map[string]any {
	return map[string]any{
		"id":               ext.ID,
		"slug":             ext.Slug,
		"name":             ext.Name,
		"version":          ext.Version,
		"description":      ext.Description,
		"state":            ext.State,
		"entrypoints":      ext.Entrypoints,
		"capabilities":     ext.Capabilities,
		"last_error":       ext.LastError,
		"auto_sync_models": ext.AutoSyncModels,
		"model_count":      modelCount,
		"installed_at":     ext.InstalledAt,
		"updated_at":       ext.UpdatedAt,
	}
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// parseBoolDefault parses common truthy/falsey strings; empty/unknown → def.
func parseBoolDefault(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "yes", "on":
		return true
	case "0", "f", "false", "no", "off":
		return false
	default:
		return def
	}
}

