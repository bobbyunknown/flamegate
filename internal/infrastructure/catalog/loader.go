package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultCatalogURL is the official models.dev raw catalog endpoint.
	DefaultCatalogURL = "https://models.dev/api.json"

	// DefaultSyncInterval is the default periodic sync interval (24 hours).
	DefaultSyncInterval = 24 * time.Hour

	// MinSyncInterval is the minimum allowable periodic sync interval (10 minutes).
	MinSyncInterval = 10 * time.Minute
)

// LoadCache reads cached catalog data from s.cfg.CachePath if it exists and parses it into internal indices.
// If the cache file does not exist or CachePath is empty, it returns nil gracefully.
func (s *Service) LoadCache() error {
	if s.cfg.CachePath == "" {
		return nil
	}

	data, err := os.ReadFile(s.cfg.CachePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read catalog cache %q: %w", s.cfg.CachePath, err)
	}

	if err := s.LoadFromBytes(data); err != nil {
		return fmt.Errorf("parse catalog cache %q: %w", s.cfg.CachePath, err)
	}

	return nil
}

// SyncNow fetches the latest catalog from s.cfg.URL, handling ETags and atomic cache saving.
func (s *Service) SyncNow(ctx context.Context) error {
	url := s.cfg.URL
	if url == "" {
		url = DefaultCatalogURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create catalog sync request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "FlameGate-CatalogSync/1.0")

	s.mu.RLock()
	etag := s.etag
	client := s.httpClient
	s.mu.RUnlock()

	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("execute catalog sync: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		s.mu.Lock()
		s.lastSync = time.Now()
		s.mu.Unlock()
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		bodySample, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("catalog sync failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodySample)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read catalog response body: %w", err)
	}

	if err := s.LoadFromBytes(body); err != nil {
		return fmt.Errorf("load catalog response: %w", err)
	}

	newETag := resp.Header.Get("ETag")
	now := time.Now()

	s.mu.Lock()
	s.etag = newETag
	s.lastSync = now
	s.mu.Unlock()

	if s.cfg.CachePath != "" {
		if err := writeAtomic(s.cfg.CachePath, body); err != nil {
			return fmt.Errorf("write catalog cache: %w", err)
		}
	}

	return nil
}

// Start begins a background ticker loop to periodically synchronize the catalog.
// It runs SyncNow on each tick and stops immediately when ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	interval := s.cfg.SyncInterval
	if interval <= 0 {
		interval = DefaultSyncInterval
	} else if interval < MinSyncInterval {
		interval = MinSyncInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.SyncNow(ctx)
		}
	}
}

// writeAtomic writes data to a temporary file in the same directory and atomically
// renames it to filePath, creating parent directories if necessary.
func writeAtomic(filePath string, data []byte) error {
	dir := filepath.Dir(filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %q: %w", dir, err)
		}
	}

	tmpPath := fmt.Sprintf("%s.tmp.%d", filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp file %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename %q to %q: %w", tmpPath, filePath, err)
	}

	return nil
}
