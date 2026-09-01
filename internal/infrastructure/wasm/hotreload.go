package wasm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// HotReloader polls an extension directory for .wasm file changes and
// triggers recompile + swap when a hash change is detected. Only ACTIVE
// and ERROR extensions are polled — DISABLED extensions are skipped.
type HotReloader struct {
	engine   *Engine
	dir      string
	modules  map[string]*Module // slug → Module
	interval time.Duration
	log      *logrus.Entry
	hashes   map[string]string // slug → last known SHA256
	debounce time.Duration
}

// NewHotReloader creates a new HotReloader.
func NewHotReloader(engine *Engine, dir string, modules map[string]*Module, interval time.Duration, log *logrus.Entry) *HotReloader {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	return &HotReloader{
		engine:   engine,
		dir:      dir,
		modules:  modules,
		interval: interval,
		log:      log.WithField("component", "hot-reload"),
		hashes:   make(map[string]string),
		debounce: 500 * time.Millisecond,
	}
}

// Run starts the hot-reload polling loop. It blocks until ctx is cancelled.
func (h *HotReloader) Run(ctx context.Context) {
	h.log.WithFields(logrus.Fields{"dir": h.dir, "interval": h.interval}).Info("hot-reload started")

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	// Initial scan to populate hashes.
	h.scan(ctx)

	for {
		select {
		case <-ctx.Done():
			h.log.Info("hot-reload stopped")
			return
		case <-ticker.C:
			h.scan(ctx)
		}
	}
}

// scan checks each managed extension's .wasm file for changes.
func (h *HotReloader) scan(ctx context.Context) {
	for slug, mod := range h.modules {
		state := mod.State()
		// Skip DISABLED extensions — only poll ACTIVE and ERROR.
		if state == StateDisabled || state == StateUpdating {
			continue
		}

		cm, ok := h.engine.Get(slug)
		if !ok {
			continue
		}
		_ = cm // we just need to confirm it exists

		extCfg, ok := h.engine.GetConfig(slug)
		if !ok {
			continue
		}

		// Find the .wasm file for this extension.
		wasmPath, err := findWasmFile(h.dir + "/" + slug)
		if err != nil {
			continue
		}

		hash, err := fileHash(wasmPath)
		if err != nil {
			h.log.WithError(err).Warn("hot-reload: hash failed", "slug", slug)
			continue
		}

		prevHash, hasPrev := h.hashes[slug]
		if !hasPrev {
			// First time seeing this extension — record hash, don't reload.
			h.hashes[slug] = hash
			continue
		}

		if hash == prevHash {
			continue // no change
		}

		// Hash changed — reload.
		h.log.WithFields(logrus.Fields{
			"slug": slug,
			"old":  prevHash[:8],
			"new":  hash[:8],
		}).Info("hot-reload: hash changed, reloading")

		wasmBytes, readErr := os.ReadFile(wasmPath)
		if readErr != nil {
			h.log.WithError(readErr).Warn("hot-reload: read wasm failed", "slug", slug)
			continue
		}

		// Reinstall: compile new → swap → drain → close old.
		if reinstallErr := Reinstall(h.engine, mod, slug, wasmBytes, extCfg, h.log); reinstallErr != nil {
			h.log.WithError(reinstallErr).Warn("hot-reload: reinstall failed", "slug", slug)
			// Don't update hash — retry on next poll.
			continue
		}

		h.hashes[slug] = hash
		h.log.WithField("slug", slug).Info("hot-reload: extension reloaded")
	}
}

// RecordHash manually records a hash for an extension (used after initial install).
func (h *HotReloader) RecordHash(slug, hash string) {
	h.hashes[slug] = hash
}

// RemoveHash removes a tracked hash (used on uninstall).
func (h *HotReloader) RemoveHash(slug string) {
	delete(h.hashes, slug)
}

// fileHash computes the SHA256 hex digest of a file.
func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
