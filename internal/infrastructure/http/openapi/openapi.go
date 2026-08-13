// Package openapi contains the Huma v2 OpenAPI configuration for FlameGate.
package openapi

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
)

// BearerSecurity is the OpenAPI security requirement for admin endpoints.
// Supports both manual Bearer token and OAuth2 password flow in Scalar.
var BearerSecurity = []map[string][]string{
	{"bearerAuth": {}},
	{"oauth2": {}},
}

// PublicSecurity declares that an endpoint requires no authentication.
var PublicSecurity = []map[string][]string{{}}

// newHumaConfig builds the OpenAPI configuration for FlameGate.
func newHumaConfig(version, serverURL string) huma.Config {
	cfg := huma.DefaultConfig("FlameGate", version)
	cfg.Info.Description = "FlameGate LLM Proxy/Router API"
	cfg.Info.Contact = &huma.Contact{
		Name: "FlameGate",
		URL:  "https://github.com/bobbyunknown/flamegate",
	}
	cfg.DocsPath = ""
	cfg.SchemasPath = ""
	if serverURL != "" {
		cfg.Servers = []*huma.Server{{URL: serverURL}}
	}
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearerAuth": {
			Type:         "http",
			Scheme:       "bearer",
			BearerFormat: "JWT",
			Description:  "Paste a JWT token (from POST /api/auth/login or /api/auth/token)",
		},
		"oauth2": {
			Type:        "oauth2",
			Description: "Login with username and password to get a JWT token automatically",
			Flows: &huma.OAuthFlows{
				Password: &huma.OAuthFlow{
					TokenURL: "/api/auth/token",
				},
			},
		},
	}
	return cfg
}

// RegisterDocs serves /openapi.json and /docs on the chi router.
func RegisterDocs(r chi.Router, spec *huma.OpenAPI) {
	r.Get("/openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(spec)
	})

	r.Get("/docs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		//nolint:errcheck // best-effort write
		_, _ = fmt.Fprint(w, scalarDocsHTML)
	})
}

// scalarDocsHTML is the Scalar API docs page.
const scalarDocsHTML = `<!doctype html>
<html>
  <head>
    <title>FlameGate API</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <div id="app"></div>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
      Scalar.createApiReference('#app', {
        url: '/openapi.json',
        theme: 'default',
        persistAuth: true,
        authentication: {
          preferredSecurityScheme: 'oauth2',
          securitySchemes: {
            oauth2: {
              flows: {
                password: {
                  tokenUrl: '/api/auth/token',
                  'x-scalar-client-id': 'scalar-docs'
                }
              }
            }
          }
        }
      })
    </script>
  </body>
</html>`
