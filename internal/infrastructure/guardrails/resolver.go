package guardrails

import (
	"context"
	"sync"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// Resolver loads per-scope policies from the store and merges them into a
// single effective Policy. Lookups are cached for cacheTTL because the merge
// is hot-path work (every request goes through it) but policy rows change
// rarely; callers invalidate on Upsert/Delete.
type Resolver struct {
	repo     resolverStore
	cacheTTL time.Duration

	mu    sync.RWMutex
	cache map[string]resolverEntry
}

type resolverStore interface {
	GetByScope(ctx context.Context, tenantID string, scope schema.GuardrailScope, scopeID string) (schema.GuardrailPolicy, error)
}

// resolverEntry is the cached lookup result for one scope tuple. notFound
// distinguishes a negative cache hit (no row exists) from a real entry.
type resolverEntry struct {
	row      schema.GuardrailPolicy
	expires  time.Time
	notFound bool
}

// NewResolver builds a resolver bound to the given store-side repo. A TTL of
// zero disables caching (useful in tests).
func NewResolver(repo resolverStore, cacheTTL time.Duration) *Resolver {
	return &Resolver{
		repo:     repo,
		cacheTTL: cacheTTL,
		cache:    make(map[string]resolverEntry),
	}
}

// Key carries the five scope dimensions a request resolves against. Empty
// fields skip the corresponding layer.
type Key struct {
	TenantID string
	Provider string
	Model    string
	ChainID  string
	APIKeyID string
}

// Effective returns the merged policy for the given context. Layers are
// applied least-specific first so later layers override earlier ones.
func (r *Resolver) Effective(ctx context.Context, k Key) Policy {
	out := Policy{}
	lookups := []struct {
		scope schema.GuardrailScope
		id    string
	}{
		{schema.GuardrailScopeGlobal, ""},
		{schema.GuardrailScopeProvider, k.Provider},
		{schema.GuardrailScopeModel, k.Model},
		{schema.GuardrailScopeChain, k.ChainID},
		{schema.GuardrailScopeAPIKey, k.APIKeyID},
	}
	for _, l := range lookups {
		if l.scope != schema.GuardrailScopeGlobal && l.id == "" {
			continue
		}
		row, ok := r.fetch(ctx, k.TenantID, l.scope, l.id)
		if !ok || !row.Enabled {
			continue
		}
		layer, err := UnmarshalPolicy(row.Config)
		if err != nil {
			continue
		}
		out = Merge(out, layer)
	}
	return out
}

func (r *Resolver) fetch(ctx context.Context, tenantID string, scope schema.GuardrailScope, scopeID string) (schema.GuardrailPolicy, bool) {
	key := cacheKey(tenantID, scope, scopeID)

	if r.cacheTTL > 0 {
		r.mu.RLock()
		entry, ok := r.cache[key]
		r.mu.RUnlock()
		if ok && time.Now().Before(entry.expires) {
			if entry.notFound {
				return schema.GuardrailPolicy{}, false
			}
			return entry.row, true
		}
	}

	row, err := r.repo.GetByScope(ctx, tenantID, scope, scopeID)
	if err != nil {
		r.store(key, schema.GuardrailPolicy{}, true)
		return schema.GuardrailPolicy{}, false
	}
	r.store(key, row, false)
	return row, true
}

func (r *Resolver) store(key string, row schema.GuardrailPolicy, notFound bool) {
	if r.cacheTTL <= 0 {
		return
	}
	r.mu.Lock()
	r.cache[key] = resolverEntry{
		row:      row,
		expires:  time.Now().Add(r.cacheTTL),
		notFound: notFound,
	}
	r.mu.Unlock()
}

// Invalidate clears the cache entry for a scope tuple. Admin handlers must
// call this on any Upsert / Delete so the next request sees the change.
func (r *Resolver) Invalidate(tenantID string, scope schema.GuardrailScope, scopeID string) {
	key := cacheKey(tenantID, scope, scopeID)
	r.mu.Lock()
	delete(r.cache, key)
	r.mu.Unlock()
}

// InvalidateAll empties the cache. Used after bulk imports or test setup.
func (r *Resolver) InvalidateAll() {
	r.mu.Lock()
	r.cache = make(map[string]resolverEntry)
	r.mu.Unlock()
}

func cacheKey(tenantID string, scope schema.GuardrailScope, scopeID string) string {
	return tenantID + "|" + string(scope) + "|" + scopeID
}
