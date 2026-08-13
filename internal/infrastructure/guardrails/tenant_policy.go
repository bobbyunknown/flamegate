package guardrails

import (
	"context"
	"strings"
	"sync"
	"time"
)

// SettingsReader is the minimal persistence.SettingsRepo surface the tenant policy
// loader needs. Defined as an interface to keep package dependencies sparse
// and to make tests easy.
type SettingsReader interface {
	Get(ctx context.Context, key string) (string, error)
}

// SettingsTenantPolicy implements TenantPolicy by reading per-tenant flags
// from the generic settings key/value store. Keys follow:
//
//	guardrails.allow_external_engines               (global default)
//	guardrails.allow_external_engines:<tenantID>    (per-tenant override)
//
// Values are case-insensitive "true"/"false" (anything else falls back to
// the global default → true). Reads are cached for 30s so the hot path
// avoids hitting the DB on every request.
type SettingsTenantPolicy struct {
	settings SettingsReader
	ttl      time.Duration

	mu    sync.RWMutex
	cache map[string]externalEnginesEntry
}

type externalEnginesEntry struct {
	allow   bool
	expires time.Time
}

// NewSettingsTenantPolicy builds a TenantPolicy backed by the settings store.
// A nil settings reader produces a policy that always returns true.
func NewSettingsTenantPolicy(settings SettingsReader, ttl time.Duration) *SettingsTenantPolicy {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &SettingsTenantPolicy{
		settings: settings,
		ttl:      ttl,
		cache:    make(map[string]externalEnginesEntry),
	}
}

// AllowExternalEngines reports whether the given tenant may route detector
// requests to external services (OpenAI, Presidio, Bedrock, ...). When the
// flag is unset, the default is true so a fresh install keeps the engines
// the operator wired in via env vars functional.
func (p *SettingsTenantPolicy) AllowExternalEngines(ctx context.Context, tenantID string) bool {
	if p == nil || p.settings == nil {
		return true
	}
	if v, ok := p.lookup(ctx, tenantKey(tenantID)); ok {
		return v
	}
	if v, ok := p.lookup(ctx, "guardrails.allow_external_engines"); ok {
		return v
	}
	return true
}

func (p *SettingsTenantPolicy) lookup(ctx context.Context, key string) (bool, bool) {
	p.mu.RLock()
	if entry, ok := p.cache[key]; ok && time.Now().Before(entry.expires) {
		p.mu.RUnlock()
		return entry.allow, true
	}
	p.mu.RUnlock()

	raw, err := p.settings.Get(ctx, key)
	if err != nil || raw == "" {
		// Don't cache the negative result — settings keys are written
		// rarely and an in-progress write would otherwise wait 30s to
		// take effect.
		return false, false
	}
	allow := parseAllow(raw)
	p.mu.Lock()
	p.cache[key] = externalEnginesEntry{allow: allow, expires: time.Now().Add(p.ttl)}
	p.mu.Unlock()
	return allow, true
}

// Invalidate clears the cached value for the given tenant; the next call
// re-reads from the settings store. Called from the admin endpoint that
// flips the flag.
func (p *SettingsTenantPolicy) Invalidate(tenantID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	delete(p.cache, tenantKey(tenantID))
	delete(p.cache, "guardrails.allow_external_engines")
	p.mu.Unlock()
}

func tenantKey(tenantID string) string {
	if tenantID == "" {
		return "guardrails.allow_external_engines"
	}
	return "guardrails.allow_external_engines:" + tenantID
}

func parseAllow(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}
