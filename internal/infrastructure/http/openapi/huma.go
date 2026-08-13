package openapi

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// NewHumaAPI creates a Huma API on a Chi subrouter mounted at /api.
// rawRoutes registers raw Chi routes, such as SSE and OAuth2 form endpoints,
// before Huma takes over typed REST routing.
func NewHumaAPI(r chi.Router, version, serverURL string, rawRoutes func(chi.Router)) huma.API {
	sub := chi.NewRouter()
	sub.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), httpRequestKey, r)
			ctx = context.WithValue(ctx, httpResponseKey, w)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})
	if rawRoutes != nil {
		rawRoutes(sub)
	}
	api := humachi.New(sub, newHumaConfig(version, serverURL+"/api"))
	r.Mount("/api", sub)
	return api
}
