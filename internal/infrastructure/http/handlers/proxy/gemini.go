package proxy

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/pipeline"
)

// handleGeminiGenerate serves Gemini's native generateContent endpoint:
//
//	POST /v1beta/models/{model}:generateContent
//	POST /v1beta/models/{model}:streamGenerateContent
//
// Gemini SDK clients embed the model and action in the URL path rather than the
// body. This handler extracts both, parses the Gemini-format body, sets the
// model, and runs the same chat pipeline as the OpenAI/Anthropic edges —
// translating Gemini -> canonical -> the chosen provider dialect. The model
// string still flows through chain/provider resolution, so a Gemini client can
// target any FlameGate chain or provider/model.
func (s *Handler) HandleGeminiGenerate(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	tenantID := tenantOf(key)

	// Path param is "{model}:{action}", e.g. "gemini-2.5-flash:generateContent".
	modelAction := chi.URLParam(r, "modelAction")
	model, action, ok := strings.Cut(modelAction, ":")
	if !ok || model == "" {
		WriteError(w, http.StatusBadRequest, "expected /v1beta/models/{model}:generateContent")
		return
	}
	stream := strings.HasPrefix(action, "stream")

	codec, err := s.codecs.Codec(core.DialectGemini)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "unsupported dialect")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	req, err := codec.ParseRequest(body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	// The Gemini body carries no model; it comes from the URL path.
	req.Model = model
	req.Stream = stream

	req.Metadata = core.RequestMetadata{
		ClientKind:    detectClient(r),
		SourceDialect: core.DialectGemini,
		APIKeyID:      key.ID,
		TenantID:      tenantID,
		ProjectID:     key.ProjectID,
		RequestID:     chimiddleware.GetReqID(r.Context()),
	}

	resolved, err := resolveTargets(r.Context(), s.chains, s.aliases, s.latencyReader(), tenantID, req.Model)
	if err != nil {
		var bad badModelError
		if errors.As(err, &bad) {
			WriteError(w, http.StatusBadRequest, bad.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, "failed to resolve model")
		return
	}
	if len(resolved.Targets) > 0 {
		req.Metadata.Provider = resolved.Targets[0].Provider
	}
	req.Metadata.ChainID = resolved.PlanOpts.ChainID
	affinityKey := requestAffinityKey(r, req)
	req.Metadata.ContextAffinityKey = affinityKey

	opts := pipeline.Options{
		Targets:  resolved.Targets,
		PlanOpts: s.endpointPlanOptions(r.Context(), resolved.PlanOpts, resolved.Targets, affinityKey),
		Slimmer:  s.slimmerConfig(),
		Terse:    s.terseConfig(),
		Caveman:  s.cavemanConfig(),
		Headroom: s.headroomConfig(),
		Ponytail: s.ponytailConfig(),
	}

	if req.Stream {
		s.streamChat(w, r, codec, req, opts, key.Name)
		return
	}
	s.unaryChat(w, r, codec, req, opts, key.Name)
}
