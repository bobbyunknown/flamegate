package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/handlers"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/middleware"
)

func (s *Server) mountProxyAPI(r chi.Router) {
	r.Get("/v1", func(w http.ResponseWriter, _ *http.Request) {
		handlers.WriteJSON(w, http.StatusOK, map[string]any{
			"name":    "FlameGate",
			"version": s.proxyHandler.VersionString(),
			"status":  "ok",
			"endpoints": []string{
				"/v1/chat/completions", "/v1/messages", "/v1/responses",
				"/v1/models", "/v1/embeddings", "/v1/images/generations",
				"/v1/audio/speech", "/v1/audio/transcriptions",
				"/v1/search", "/v1/web/fetch",
			},
		})
	})

	// Public portal (no auth)
	r.Get("/v1/portal/keys/{id}/usage", s.proxyHandler.HandlePortalKeyUsage)
	r.Get("/v1/portal/branding", s.proxyHandler.PortalBranding)

	r.Group(func(r chi.Router) {
		maxConc := s.cfg.Server.MaxConcurrentRequests
		if maxConc <= 0 {
			maxConc = 100
		}
		r.Use(middleware.ConcurrencyLimiter(maxConc))
		r.Use(middleware.APIKeyAuth(
			s.proxyHandler.Identity(),
			s.proxyHandler.Logger(),
			s.proxyHandler.ConsoleLog(),
		))

		r.Post("/v1/chat/completions", s.proxyHandler.HandleOpenAIChat)
		r.Post("/v1/messages", s.proxyHandler.HandleAnthropicMessages)
		r.Post("/v1/messages/count_tokens", s.proxyHandler.HandleAnthropicCountTokens)
		r.Post("/v1/responses", s.proxyHandler.HandleOpenAIResponses)
		r.Post("/v1beta/models/{modelAction}", s.proxyHandler.HandleGeminiGenerate)

		r.Post("/v1/embeddings", s.proxyHandler.HandleEmbeddings)
		r.Post("/v1/images/generations", s.proxyHandler.HandleImageGeneration)
		r.Post("/v1/audio/speech", s.proxyHandler.HandleAudioSpeech)
		r.Post("/v1/audio/transcriptions", s.proxyHandler.HandleAudioTranscription)
		r.Post("/v1/search", s.proxyHandler.HandleWebSearch)
		r.Post("/v1/web/fetch", s.proxyHandler.HandleWebFetch)

		r.Get("/v1/models", s.proxyHandler.HandleListModels)
		r.Get("/v1/models/info", s.proxyHandler.HandleModelInfo)
		r.Get("/v1/models/{kind}", s.proxyHandler.HandleListModelsByKind)

		r.Get("/v1/keys/me/usage", s.proxyHandler.HandleKeyUsage)
	})
}
