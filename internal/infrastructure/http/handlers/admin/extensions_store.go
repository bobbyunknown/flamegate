package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/extstore"
)

// MountStore registers the extensions store surface (catalog browse + remote
// install/update). All persistence work is delegated to s.extStore.
func (s *Handler) MountStore(r chi.Router) {
	r.Get("/extensions/store", s.adminStoreList)
	r.Get("/extensions/store/{slug}", s.adminStoreGet)
	r.Post("/extensions/store/install", s.adminStoreInstall)
	r.Post("/extensions/{slug}/update", s.adminStoreUpdate)
}

// adminStoreList returns the store catalog with live version info.
func (s *Handler) adminStoreList(w http.ResponseWriter, r *http.Request) {
	if s.extStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension store not configured")
		return
	}
	items, err := s.extStore.ListStore(r.Context())
	if err != nil {
		WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"extensions": items})
}

// adminStoreGet returns a single catalog entry with live version.
func (s *Handler) adminStoreGet(w http.ResponseWriter, r *http.Request) {
	if s.extStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension store not configured")
		return
	}
	slug := chi.URLParam(r, "slug")
	item, err := s.extStore.GetStore(r.Context(), slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

// adminStoreInstall installs a store/github/url source.
// Body: {"source":"store:codex"}.
func (s *Handler) adminStoreInstall(w http.ResponseWriter, r *http.Request) {
	if s.extStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension store not configured")
		return
	}
	var body struct {
		Source string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Source == "" {
		WriteError(w, http.StatusBadRequest, "source is required")
		return
	}
	res, err := s.extStore.Install(r.Context(), body.Source)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, extstore.ErrChecksumMismatch) ||
			errors.Is(err, extstore.ErrSignatureInvalid) ||
			errors.Is(err, extstore.ErrSignatureMissing) ||
			errors.Is(err, extstore.ErrUntrustedSource) ||
			errors.Is(err, extstore.ErrZipTraversal) {
			status = http.StatusUnprocessableEntity
		}
		WriteError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// adminStoreUpdate upgrades an installed extension to the latest release.
func (s *Handler) adminStoreUpdate(w http.ResponseWriter, r *http.Request) {
	if s.extStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "extension store not configured")
		return
	}
	slug := chi.URLParam(r, "slug")

	// Read the installed source so update resolves from the same origin.
	ext, err := s.db.Extensions().FindBySlug(r.Context(), slug)
	if err != nil {
		WriteError(w, http.StatusNotFound, "extension not installed")
		return
	}
	if ext.SourceURI == "" {
		WriteError(w, http.StatusBadRequest, "extension was installed locally; update only works for store/github/url sources")
		return
	}
	res, err := s.extStore.Install(r.Context(), ext.SourceURI)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}