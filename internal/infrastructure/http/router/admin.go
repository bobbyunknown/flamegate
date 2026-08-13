package router

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/middleware"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/openapi"
)

func (s *Server) mountAdminAPI(r chi.Router, version, serverURL string) huma.API {
	api := openapi.NewHumaAPI(r, version, serverURL, func(sub chi.Router) {
		// OAuth2 token endpoint is raw Chi: it must accept form-urlencoded per RFC6749.
		sub.Group(func(r chi.Router) {
			r.Use(middleware.LoopbackOnly(s.cfg))
			r.Use(middleware.LoginRateLimiter())
			r.Post("/auth/token", s.adminHandler.HandleToken)
		})

		// SSE/file/task endpoints stay raw Chi; typed CRUD is registered via Huma.
		sub.Group(func(r chi.Router) {
			r.Use(middleware.LoopbackOnly(s.cfg))
			r.Use(middleware.SessionAuth(s.adminHandler.Auth(), s.adminHandler.Logger()))
			s.adminHandler.MountAdmin(r)
		})
	})
	s.registerAdminAPI(api)
	s.registerRawAdminSpec(api)
	return api
}

// registerRawAdminSpec adds OpenAPI entries for raw Chi endpoints so Scalar shows
// them with path params / request bodies. Handlers still live in MountAdmin.
func (s *Server) registerRawAdminSpec(api huma.API) {
	spec := api.OpenAPI()
	spec.AddOperation(&huma.Operation{
		OperationID: "post-token",
		Method:      http.MethodPost,
		Path:        "/auth/token",
		Summary:     "OAuth2 token endpoint (password flow)",
		Description: "Returns JWT Bearer token for use in Scalar and API clients. application/x-www-form-urlencoded: grant_type=password, username, password.",
		Tags:        []string{"Auth"},
		Security:    []map[string][]string{{}},
	})

	slugParam := &huma.Param{
		Name:     "slug",
		In:       "path",
		Required: true,
		Description: "Extension slug (unique id from schema.json, e.g. xiaomi-mimo). Same as extensions.slug.",
		Schema:   &huma.Schema{Type: huma.TypeString, Examples: []any{"xiaomi-mimo"}},
	}

	// GET /extensions
	spec.AddOperation(&huma.Operation{
		OperationID: "listExtensions",
		Method:      http.MethodGet,
		Path:        "/extensions",
		Summary:     "List installed extensions",
		Description: "Returns all WASM extensions for the admin tenant, including auto_sync_models and model_count.",
		Tags:        []string{"Extensions"},
		Security:    openapi.BearerSecurity,
	})

	// POST /extensions/install (multipart)
	spec.AddOperation(&huma.Operation{
		OperationID: "installExtension",
		Method:      http.MethodPost,
		Path:        "/extensions/install",
		Summary:     "Install extension",
		Description: "Multipart form: wasm (.wasm file), schema (schema.json). Optional auto_sync_models=true|false (default true).",
		Tags:        []string{"Extensions"},
		Security:    openapi.BearerSecurity,
		RequestBody: &huma.RequestBody{
			Required: true,
			Content: map[string]*huma.MediaType{
				"multipart/form-data": {
					Schema: &huma.Schema{
						Type: huma.TypeObject,
						Properties: map[string]*huma.Schema{
							"wasm":             {Type: huma.TypeString, Format: "binary", Description: "Compiled .wasm module"},
							"schema":           {Type: huma.TypeString, Format: "binary", Description: "schema.json for the extension"},
							"auto_sync_models": {Type: huma.TypeBoolean, Description: "If true (default), run list_models after install", Default: true},
						},
						Required: []string{"wasm", "schema"},
					},
				},
			},
		},
	})

	// GET /extensions/{slug}
	spec.AddOperation(&huma.Operation{
		OperationID: "getExtension",
		Method:      http.MethodGet,
		Path:        "/extensions/{slug}",
		Summary:     "Get extension",
		Tags:        []string{"Extensions"},
		Security:    openapi.BearerSecurity,
		Parameters:  []*huma.Param{slugParam},
	})

	// DELETE /extensions/{slug}
	spec.AddOperation(&huma.Operation{
		OperationID: "uninstallExtension",
		Method:      http.MethodDelete,
		Path:        "/extensions/{slug}",
		Summary:     "Uninstall extension",
		Description: "Unloads WASM, deletes extension_models rows and files.",
		Tags:        []string{"Extensions"},
		Security:    openapi.BearerSecurity,
		Parameters:  []*huma.Param{slugParam},
	})

	// POST enable / disable / sync-models
	for _, op := range []struct {
		id, path, summary, desc string
	}{
		{"enableExtension", "/extensions/{slug}/enable", "Enable extension", "Sets ACTIVE and registers catalog. Auto-syncs models only if auto_sync_models is true."},
		{"disableExtension", "/extensions/{slug}/disable", "Disable extension", "Sets DISABLED and unregisters catalog."},
		{"syncExtensionModels", "/extensions/{slug}/sync-models", "Sync extension models", "Calls guest list_models and replaces discovered rows. Always works (ignores auto_sync_models flag)."},
	} {
		spec.AddOperation(&huma.Operation{
			OperationID: op.id,
			Method:      http.MethodPost,
			Path:        op.path,
			Summary:     op.summary,
			Description: op.desc,
			Tags:        []string{"Extensions"},
			Security:    openapi.BearerSecurity,
			Parameters:  []*huma.Param{slugParam},
		})
	}

	// PUT /extensions/{slug}/auto-sync-models
	spec.AddOperation(&huma.Operation{
		OperationID: "setExtensionAutoSyncModels",
		Method:      http.MethodPut,
		Path:        "/extensions/{slug}/auto-sync-models",
		Summary:     "Set auto-sync models flag",
		Description: "Controls whether install/enable auto-run list_models into extension_models. Manual sync-models always available.",
		Tags:        []string{"Extensions"},
		Security:    openapi.BearerSecurity,
		Parameters:  []*huma.Param{slugParam},
		RequestBody: &huma.RequestBody{
			Required:    true,
			Description: "Toggle auto discovery on install/enable",
			Content: map[string]*huma.MediaType{
				"application/json": {
					Schema: &huma.Schema{
						Type: huma.TypeObject,
						Properties: map[string]*huma.Schema{
							"auto_sync_models": {
								Type:        huma.TypeBoolean,
								Description: "true = auto-sync on install/enable; false = only manual sync",
								Examples:    []any{true, false},
							},
						},
						Required: []string{"auto_sync_models"},
					},
					Example: map[string]any{"auto_sync_models": true},
				},
			},
		},
	})
}
