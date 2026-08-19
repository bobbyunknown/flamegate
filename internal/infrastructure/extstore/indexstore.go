package extstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SourceRef describes where an extension's releases live.
type SourceRef struct {
	Type         string `json:"type"` // "github"
	Owner        string `json:"owner"`
	Repo         string `json:"repo"`
	TagPrefix    string `json:"tag_prefix"`
	AssetPattern string `json:"asset_pattern"`
}

// IndexExt is a single catalog entry in store/index.json.
type IndexExt struct {
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Source      SourceRef `json:"source"`
}

// StoreIndex is the root of store/index.json.
type StoreIndex struct {
	Version    int        `json:"version"`
	IndexURL   string     `json:"index_url,omitempty"`
	Extensions []IndexExt `json:"extensions"`
}

// IndexStore fetches and caches store/index.json.
type IndexStore struct {
	httpc *http.Client
	cache *TTLCache
	ttl   int64 // seconds; unused placeholder kept for symmetry
}

// NewIndexStore builds an index store with the supplied HTTP client.
func NewIndexStore(httpc *http.Client, cache *TTLCache) *IndexStore {
	return &IndexStore{httpc: httpc, cache: cache}
}

// Fetch downloads and parses the index at indexURL. Results are cached keyed by
// the URL for the store's configured TTL.
func (s *IndexStore) Fetch(ctx context.Context, indexURL string) (*StoreIndex, error) {
	key := "index:" + indexURL
	if v, ok := s.cache.Get(key); ok {
		return v.(*StoreIndex), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("index: build request: %w", err)
	}
	res, err := s.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreIndexNotFound, err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fmt.Errorf("%w: %d %s", ErrStoreIndexNotFound, res.StatusCode, res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("index: read: %w", err)
	}
	var idx StoreIndex
	if err := json.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("index: parse: %w", err)
	}
	idx.IndexURL = indexURL
	s.cache.Set(key, &idx, s.ttlDuration())
	return &idx, nil
}

// Find looks up a slug in the index.
func (idx *StoreIndex) Find(slug string) (IndexExt, error) {
	for _, e := range idx.Extensions {
		if e.Slug == slug {
			return e, nil
		}
	}
	return IndexExt{}, fmt.Errorf("%w: %s", ErrExtensionNotFound, slug)
}

func (s *IndexStore) ttlDuration() time.Duration { return 10 * time.Minute }