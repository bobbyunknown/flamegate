package catalog

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/meter"
)

// Config defines configuration options for the catalog service.
type Config struct {
	Enabled      bool          `json:"enabled,omitempty"`
	URL          string        `json:"url,omitempty"`
	CachePath    string        `json:"cache_path,omitempty"`
	AutoSync     bool          `json:"auto_sync,omitempty"`
	SyncInterval time.Duration `json:"sync_interval,omitempty"`
	Timeout      time.Duration `json:"timeout,omitempty"`
}

// Service manages model catalog lookup, alias resolution, and pricing.
type Service struct {
	cfg             Config
	mu              sync.RWMutex
	byProviderModel map[string]*ModelSpec
	canonicalIndex  map[string]*ModelSpec
	allModels       []ModelSpec
	etag            string
	lastSync        time.Time
	httpClient      *http.Client
}

// CatalogService is an alias for Service.
type CatalogService = Service

// New creates a new catalog Service instance.
func New(cfg Config) *Service {
	return &Service{
		cfg:             cfg,
		byProviderModel: make(map[string]*ModelSpec),
		canonicalIndex:  make(map[string]*ModelSpec),
		allModels:       []ModelSpec{},
		httpClient:      &http.Client{Timeout: 30 * time.Second},
	}
}

// NewService creates a new catalog Service instance.
func NewService(cfg Config) *Service {
	return New(cfg)
}

// ETag returns the current cached catalog ETag.
func (s *Service) ETag() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.etag
}

// LastSync returns the timestamp of the last successful catalog sync.
func (s *Service) LastSync() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSync
}

// SetHTTPClient configures a custom HTTP client for remote catalog syncing.
func (s *Service) SetHTTPClient(client *http.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.httpClient = client
}

// LoadFromBytes unmarshals models.dev raw JSON data and populates internal indices.
func (s *Service) LoadFromBytes(data []byte) error {
	byProviderModel, canonicalIndex, err := ParseRawCatalog(data)
	if err != nil {
		return fmt.Errorf("load catalog from bytes: %w", err)
	}

	allModels := make([]ModelSpec, 0, len(byProviderModel))
	for _, spec := range byProviderModel {
		if spec != nil {
			allModels = append(allModels, *spec)
		}
	}

	// Deterministic sorting by Provider then ID
	sort.Slice(allModels, func(i, j int) bool {
		if allModels[i].Provider == allModels[j].Provider {
			return allModels[i].ID < allModels[j].ID
		}
		return allModels[i].Provider < allModels[j].Provider
	})

	s.mu.Lock()
	defer s.mu.Unlock()
	s.byProviderModel = byProviderModel
	s.canonicalIndex = canonicalIndex
	s.allModels = allModels

	return nil
}

// FindModel looks up a model specification using a 3-tier fallback resolution:
// 1. Exact Provider + Model Match (e.g. "google/gemini-2.5-flash")
// 2. Alias Provider Match (e.g. "antigravity" -> "google", then "google/gemini-2.5-flash")
// 3. Canonical Base Model Slug Match via canonicalIndex[ExtractBaseModelSlug(modelID)]
func (s *Service) FindModel(provider, modelID string) (*ModelSpec, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	normProvider := strings.ToLower(strings.TrimSpace(provider))
	normModelID := strings.TrimSpace(modelID)
	if normModelID == "" {
		return nil, false
	}

	// 1) Tier 1: Exact Provider + Model Match
	if normProvider != "" {
		if spec, ok := s.byProviderModel[normProvider+"/"+normModelID]; ok {
			return spec, true
		}
		if spec, ok := s.byProviderModel[normProvider+"/"+strings.ToLower(normModelID)]; ok {
			return spec, true
		}
	}
	if strings.Contains(normModelID, "/") {
		if spec, ok := s.byProviderModel[strings.ToLower(normModelID)]; ok {
			return spec, true
		}
	}

	// 2) Tier 2: Alias Provider Match
	if normProvider != "" {
		aliasProvider := ResolveProviderAlias(normProvider)
		if aliasProvider != "" && aliasProvider != normProvider {
			if spec, ok := s.byProviderModel[aliasProvider+"/"+normModelID]; ok {
				return spec, true
			}
			if spec, ok := s.byProviderModel[aliasProvider+"/"+strings.ToLower(normModelID)]; ok {
				return spec, true
			}
			baseSlug := ExtractBaseModelSlug(normModelID)
			if baseSlug != "" {
				if spec, ok := s.byProviderModel[aliasProvider+"/"+baseSlug]; ok {
					return spec, true
				}
			}
		}
	}

	// 3) Tier 3: Canonical Base Model Slug Match
	baseSlug := ExtractBaseModelSlug(normModelID)
	if baseSlug != "" {
		if spec, ok := s.canonicalIndex[baseSlug]; ok {
			return spec, true
		}
	}

	return nil, false
}

// GetPrice returns the per-million-token Price for a given provider and model.
// Returns (zero Price, false) if the model cannot be resolved in the catalog.
func (s *Service) GetPrice(provider, modelID string) (meter.Price, bool) {
	spec, ok := s.FindModel(provider, modelID)
	if !ok || spec == nil {
		return meter.Price{}, false
	}

	return meter.Price{
		InputPerM:       spec.Cost.Input,
		OutputPerM:      spec.Cost.Output,
		CachedInputPerM: spec.Cost.CacheRead,
		CacheWritePerM:  spec.Cost.CacheWrite,
		ReasoningPerM:   spec.Cost.Reasoning,
	}, true
}

// ListAllModels returns a copy of all loaded model specifications.
func (s *Service) ListAllModels() []ModelSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]ModelSpec, len(s.allModels))
	copy(res, s.allModels)
	return res
}
