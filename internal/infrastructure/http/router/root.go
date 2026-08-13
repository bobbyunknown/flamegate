// Package router wires the chi router, middleware, and handlers into a
// working HTTP server. It owns route registration and middleware ordering.
package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/handlers"
	adminhandler "github.com/bobbyunknown/flamegate/internal/infrastructure/http/handlers/admin"
	proxyhandler "github.com/bobbyunknown/flamegate/internal/infrastructure/http/handlers/proxy"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/middleware"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/openapi"
)

// Server is the top-level HTTP server that owns the root chi router.
type Server struct {
	Router       chi.Router
	adminHandler *adminhandler.Handler
	proxyHandler *proxyhandler.Handler
	cfg          config.Config
	log          *logrus.Logger
}

// New creates a Server, wires middleware and routes, and returns it.
func New(d handlers.Deps) *Server {
	cfg := d.Config
	log := d.Logger
	if log == nil {
		log = logrus.StandardLogger()
	}

	s := &Server{
		adminHandler: adminhandler.New(d),
		proxyHandler: proxyhandler.New(d),
		cfg:          cfg,
		log:          log,
	}
	r := chi.NewRouter()
	s.Router = r

	// Global middleware MUST be registered before any routes (Chi v5 rule).
	s.registerMiddleware(r)
	s.registerRoutes(r)
	// When server.proxy_port is set, /v1 lives only on that dedicated listener.
	// Otherwise admin + proxy share server.port.
	if cfg.ProxyAddr() == "" {
		s.mountProxyAPI(r)
	}
	api := s.mountAdminAPI(r, d.Version, "http://"+cfg.Addr())
	if cfg.Docs.Enabled {
		openapi.RegisterDocs(r, api.OpenAPI())
	}
	return s
}

// Handler returns the root http.Handler (admin + proxy on same tree).
func (s *Server) Handler() http.Handler { return s.Router }

// ProxyHandler returns a handler that serves only the client/proxy API (/v1).
// Used when server.proxy_port is set so /v1 can bind a dedicated port.
func (s *Server) ProxyHandler() http.Handler {
	r := chi.NewRouter()
	// Same global middleware subset needed by proxy clients (no admin CORS cookies).
	r.Use(middleware.CollapseDoubleV1)
	r.Use(chimw.RequestID)
	r.Use(middleware.RequestLogging(s.log))
	r.Use(middleware.PanicRecovery())
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc: func(r *http.Request, origin string) bool { return true },
		AllowedMethods:  []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Authorization", "Content-Type", "x-api-key",
			"X-FlameGate-Affinity", "X-Conversation-ID", "X-Thread-ID",
			"X-Session-ID", "OpenAI-Conversation-ID",
		},
		AllowCredentials: false,
	}))
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		handlers.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "surface": "proxy"})
	})
	s.mountProxyAPI(r)
	return r
}

func (s *Server) registerMiddleware(r chi.Router) {
	r.Use(middleware.CollapseDoubleV1)
	r.Use(chimw.RequestID)
	r.Use(middleware.RequestLogging(s.log))
	r.Use(middleware.PanicRecovery())
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc: func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Authorization", "Content-Type", "x-api-key",
			"X-FlameGate-Affinity", "X-Conversation-ID", "X-Thread-ID",
			"X-Session-ID", "OpenAI-Conversation-ID",
		},
		AllowCredentials: true,
	}))
}

// registerRoutes registers root routes that are not part of /v1 or /api.
func (s *Server) registerRoutes(r chi.Router) {
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		handlers.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	if s.adminHandler.Metrics() != nil {
		r.Group(func(r chi.Router) {
			r.Use(middleware.LoopbackOnly(s.cfg))
			r.Handle("/metrics", promhttp.HandlerFor(s.adminHandler.Metrics().Registry(), promhttp.HandlerOpts{}))
		})
	}

}
