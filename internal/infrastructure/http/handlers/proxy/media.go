package proxy

import (
	"encoding/json"
	"io"
	"net/http"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/dispatch"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/pipeline"
	"github.com/bobbyunknown/flamegate/internal/shared/httputil"
)

// This file implements the gateway edge for the non-chat capabilities. Each
// handler parses an OpenAI-style request, resolves the routing target from the
// model string, runs the matching pipeline method, and renders the response.

// mediaOptions builds pipeline.MediaOptions from a resolved request: it maps
// the model string to fallback targets and attaches tenant/scope metadata.
func (s *Handler) mediaOptions(r *http.Request, model string) (pipeline.MediaOptions, error) {
	key, _ := authedKey(r.Context())
	tenantID := tenantOf(key)
	resolved, err := resolveTargets(r.Context(), s.chains, s.aliases, s.latencyReader(), tenantID, model)
	if err != nil {
		return pipeline.MediaOptions{}, err
	}
	effectiveLimits, err := s.effectiveLimits(r.Context(), key)
	if err != nil {
		return pipeline.MediaOptions{}, err
	}
	return pipeline.MediaOptions{
		Targets:   resolved.Targets,
		TenantID:  tenantID,
		ProjectID: key.ProjectID,
		APIKeyID:  key.ID,
		Limits:    effectiveLimits,
	}, nil
}

// readJSON reads and decodes a JSON request body into v.
func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "failed to read request body")
		return false
	}
	if err := json.Unmarshal(body, v); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return false
	}
	return true
}

// writeMediaError maps a pipeline/provider error (or bad-model error) to HTTP.
func (s *Handler) writeMediaError(w http.ResponseWriter, err error) {
	var bad badModelError
	if asBadModel(err, &bad) {
		WriteError(w, http.StatusBadRequest, bad.Error())
		return
	}
	s.writeProviderError(w, err)
}

// ---- Embeddings -------------------------------------------------------------

func (s *Handler) HandleEmbeddings(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	var body struct {
		Model      string          `json:"model"`
		Input      json.RawMessage `json:"input"`
		Dimensions int             `json:"dimensions"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	if body.Model == "" {
		WriteError(w, http.StatusBadRequest, "model is required")
		return
	}
	inputs, err := decodeEmbeddingInput(body.Input)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	opts, err := s.mediaOptions(r, body.Model)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	model := modelTail(opts.Targets)
	resp, provider, perr := s.pipeline.Embeddings(r.Context(), &core.EmbeddingRequest{
		Model: model, Input: inputs, Dimensions: body.Dimensions,
	}, opts)
	if perr != nil {
		s.logRequest(key.Name, provider, model, 0, 0, 0, false, perr)
		s.writeMediaError(w, perr)
		return
	}
	s.logRequest(key.Name, provider, model, resp.Usage.TotalTokens, 0, 0, false, nil)

	data := make([]map[string]any, 0, len(resp.Vectors))
	for i, v := range resp.Vectors {
		data = append(data, map[string]any{"object": "embedding", "index": i, "embedding": v})
	}
	w.Header().Set("X-FlameGate-Provider", provider)
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list", "model": resp.Model, "data": data,
		"usage": map[string]any{
			"prompt_tokens": resp.Usage.PromptTokens, "total_tokens": resp.Usage.TotalTokens,
		},
	})
}

// decodeEmbeddingInput accepts a string or an array of strings.
func decodeEmbeddingInput(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, errBadModel("input is required")
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}, nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, nil
	}
	return nil, errBadModel("input must be a string or array of strings")
}

// ---- Image generation -------------------------------------------------------

func (s *Handler) HandleImageGeneration(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	var req core.ImageRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.Model == "" || req.Prompt == "" {
		WriteError(w, http.StatusBadRequest, "model and prompt are required")
		return
	}
	opts, err := s.mediaOptions(r, req.Model)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	req.Model = modelTail(opts.Targets)
	resp, provider, perr := s.pipeline.GenerateImage(r.Context(), &req, opts)
	if perr != nil {
		s.logRequest(key.Name, provider, req.Model, 0, 0, 0, false, perr)
		s.writeMediaError(w, perr)
		return
	}
	s.logRequest(key.Name, provider, req.Model, 0, 0, 0, false, nil)
	w.Header().Set("X-FlameGate-Provider", provider)
	writeJSON(w, http.StatusOK, map[string]any{"created": resp.Created, "data": resp.Data})
}

// ---- Speech-to-text ---------------------------------------------------------

func (s *Handler) HandleAudioTranscription(w http.ResponseWriter, r *http.Request) {
	// Accept multipart/form-data (OpenAI standard) with a file + model field.
	if err := r.ParseMultipartForm(maxBodyBytes); err != nil {
		WriteError(w, http.StatusBadRequest, "expected multipart/form-data: "+err.Error())
		return
	}
	model := r.FormValue("model")
	if model == "" {
		WriteError(w, http.StatusBadRequest, "model is required")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	audio, err := io.ReadAll(io.LimitReader(file, maxBodyBytes))
	if err != nil {
		WriteError(w, http.StatusBadRequest, "failed to read audio file")
		return
	}

	opts, err := s.mediaOptions(r, model)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	req := &core.TranscriptionRequest{
		Model:          modelTail(opts.Targets),
		Audio:          audio,
		Filename:       header.Filename,
		ContentType:    header.Header.Get("Content-Type"),
		Language:       r.FormValue("language"),
		Prompt:         r.FormValue("prompt"),
		ResponseFormat: r.FormValue("response_format"),
	}
	resp, provider, perr := s.pipeline.Transcribe(r.Context(), req, opts)
	if perr != nil {
		s.writeMediaError(w, perr)
		return
	}
	w.Header().Set("X-FlameGate-Provider", provider)
	if req.ResponseFormat == "text" {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, resp.Text)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": resp.Text, "language": resp.Language, "duration": resp.Duration})
}

// ---- Text-to-speech ---------------------------------------------------------

func (s *Handler) HandleAudioSpeech(w http.ResponseWriter, r *http.Request) {
	var req core.SpeechRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.Model == "" || req.Input == "" {
		WriteError(w, http.StatusBadRequest, "model and input are required")
		return
	}
	opts, err := s.mediaOptions(r, req.Model)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	req.Model = modelTail(opts.Targets)
	resp, provider, perr := s.pipeline.Synthesize(r.Context(), &req, opts)
	if perr != nil {
		s.writeMediaError(w, perr)
		return
	}
	w.Header().Set("Content-Type", resp.ContentType)
	w.Header().Set("X-FlameGate-Provider", provider)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.Audio)
}

// ---- Web search -------------------------------------------------------------

func (s *Handler) HandleWebSearch(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	var req core.SearchRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.Query == "" {
		WriteError(w, http.StatusBadRequest, "query is required")
		return
	}
	// Default the search provider/model when the client omits it: pick the
	// first configured search-capable target via the model string.
	model := req.Model
	if model == "" {
		WriteError(w, http.StatusBadRequest, "model is required (provider/model, e.g. tavily/tavily-search)")
		return
	}
	opts, err := s.mediaOptions(r, model)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	req.Model = modelTail(opts.Targets)
	resp, provider, perr := s.pipeline.Search(r.Context(), &req, opts)
	if perr != nil {
		s.logRequest(key.Name, provider, req.Model, 0, 0, 0, false, perr)
		s.writeMediaError(w, perr)
		return
	}
	s.logRequest(key.Name, provider, req.Model, 0, 0, 0, false, nil)
	w.Header().Set("X-FlameGate-Provider", provider)
	writeJSON(w, http.StatusOK, map[string]any{"query": resp.Query, "results": resp.Results})
}

// ---- Web fetch --------------------------------------------------------------

func (s *Handler) HandleWebFetch(w http.ResponseWriter, r *http.Request) {
	key, _ := authedKey(r.Context())
	var req core.FetchRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.URL == "" {
		WriteError(w, http.StatusBadRequest, "url is required")
		return
	}

	// SSRF Protection: Validate URL before passing to upstream services
	if err := httputil.ValidateOutboundURL(req.URL); err != nil {
		s.log.Warn("blocked suspicious web fetch URL", "url", req.URL, "error", err)
		WriteError(w, http.StatusBadRequest, "invalid url: URL blocked by security policy")
		return
	}

	model := req.Model
	if model == "" {
		WriteError(w, http.StatusBadRequest, "model is required (provider/model, e.g. firecrawl/firecrawl-scrape)")
		return
	}
	opts, err := s.mediaOptions(r, model)
	if err != nil {
		s.writeMediaError(w, err)
		return
	}
	req.Model = modelTail(opts.Targets)
	resp, provider, perr := s.pipeline.Fetch(r.Context(), &req, opts)
	if perr != nil {
		s.logRequest(key.Name, provider, req.Model, 0, 0, 0, false, perr)
		s.writeMediaError(w, perr)
		return
	}
	s.logRequest(key.Name, provider, req.Model, 0, 0, 0, false, nil)
	w.Header().Set("X-FlameGate-Provider", provider)
	writeJSON(w, http.StatusOK, map[string]any{
		"url": resp.URL, "title": resp.Title, "content": resp.Content, "format": resp.Format,
	})
}

// ---- helpers ----------------------------------------------------------------

// modelTail returns the model id of the first resolved target, used to pass the
// concrete model down to the pipeline. The pipeline overrides it per-attempt,
// but media requests usually resolve to a single provider/model target.
func modelTail(targets []dispatch.Target) string {
	if len(targets) > 0 {
		return targets[0].Model
	}
	return ""
}

// asBadModel reports whether err is a badModelError and assigns it to target.
func asBadModel(err error, target *badModelError) bool {
	if bm, ok := err.(badModelError); ok {
		*target = bm
		return true
	}
	return false
}
