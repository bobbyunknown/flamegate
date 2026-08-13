// Package cache implements FlameGate's semantic response cache.
//
// Repeated or near-identical prompts are expensive to re-run. The cache embeds
// each request's normalized prompt into a vector and, on a later request, looks
// up the most similar cached entry by cosine similarity. A hit above the
// configured threshold returns the stored response for free, turning repeated
// workloads into zero-cost, instant responses.
//
// The default backend is an in-memory store with brute-force cosine search,
// which is more than adequate for a local single-operator cache. The Store
// interface lets a Redis/vector-DB backend drop in for larger deployments
// without touching callers.
package cache

import (
	"context"
	"math"
	"sync"
	"time"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// Entry is a cached response keyed by its prompt embedding.
type Entry struct {
	Vector   []float32
	Response *core.ChatResponse
	Model    string
	StoredAt time.Time
}

// Store is the pluggable backend for cached entries.
type Store interface {
	// Nearest returns the most similar entry to vec and its cosine similarity,
	// or ok=false when the store is empty.
	Nearest(ctx context.Context, vec []float32) (Entry, float64, bool, error)
	// Put inserts an entry.
	Put(ctx context.Context, e Entry) error
	// Len reports the number of stored entries (for metrics/eviction).
	Len() int
}

// Config controls cache behavior.
type Config struct {
	Enabled             bool
	SimilarityThreshold float64
	TTL                 time.Duration
	// MaxEntries bounds the in-memory store; 0 means unlimited.
	MaxEntries int
}

// Cache wraps a Store with TTL, threshold, and eviction policy.
type Cache struct {
	store Store
	cfg   Config
}

// New builds a Cache over a Store. When store is nil, an in-memory store is used.
func New(cfg Config, store Store) *Cache {
	if store == nil {
		store = NewMemoryStore(cfg.MaxEntries, cfg.TTL)
	}
	if cfg.SimilarityThreshold <= 0 {
		cfg.SimilarityThreshold = 0.95
	}
	return &Cache{store: store, cfg: cfg}
}

// Lookup returns a cached response whose prompt embedding is at least the
// configured similarity to vec, or ok=false on a miss. Entries past their TTL
// are treated as misses.
func (c *Cache) Lookup(ctx context.Context, vec []float32) (*core.ChatResponse, bool, error) {
	if !c.cfg.Enabled || len(vec) == 0 {
		return nil, false, nil
	}
	entry, score, ok, err := c.store.Nearest(ctx, vec)
	if err != nil || !ok {
		return nil, false, err
	}
	if score < c.cfg.SimilarityThreshold {
		return nil, false, nil
	}
	if c.cfg.TTL > 0 && time.Since(entry.StoredAt) > c.cfg.TTL {
		return nil, false, nil
	}
	return entry.Response, true, nil
}

// Store caches a response under its prompt embedding.
func (c *Cache) Store(ctx context.Context, vec []float32, resp *core.ChatResponse) error {
	if !c.cfg.Enabled || len(vec) == 0 || resp == nil {
		return nil
	}
	return c.store.Put(ctx, Entry{
		Vector:   vec,
		Response: resp,
		Model:    resp.Model,
		StoredAt: time.Now(),
	})
}

// Enabled reports whether the cache is active.
func (c *Cache) Enabled() bool { return c.cfg.Enabled }

// MemoryStore is a brute-force in-memory vector store. Adequate for a local
// cache; swap for a vector DB at scale.
type MemoryStore struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
	cursor  int           // ring buffer write position
	count   int           // actual count (< max during warmup)
	ttl     time.Duration // used for lazy eviction during Nearest
}

// NewMemoryStore builds an in-memory store bounded by max entries (0 = unbounded).
func NewMemoryStore(max int, ttl time.Duration) *MemoryStore {
	return &MemoryStore{max: max, ttl: ttl}
}

// evict removes expired entries from the store to reclaim memory. It is
// called lazily during Put to avoid the need for a background goroutine.
func (m *MemoryStore) evict(now time.Time) {
	if m.ttl <= 0 || len(m.entries) == 0 {
		return
	}
	alive := m.entries[:0]
	for _, e := range m.entries {
		if now.Sub(e.StoredAt) <= m.ttl {
			alive = append(alive, e)
		}
	}
	removed := len(m.entries) - len(alive)
	if removed > 0 {
		m.entries = alive
		m.count -= removed
		if m.cursor >= len(m.entries) {
			m.cursor = 0
		}
	}
}

// Nearest returns the highest-cosine entry to vec.
func (m *MemoryStore) Nearest(_ context.Context, vec []float32) (Entry, float64, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.count == 0 && len(m.entries) == 0 {
		return Entry{}, 0, false, nil
	}

	now := time.Now()
	var best Entry
	bestScore := -1.0
	for _, e := range m.entries {
		if m.ttl > 0 && now.Sub(e.StoredAt) > m.ttl {
			continue // skip expired entries
		}
		s := cosine(vec, e.Vector)
		if s > bestScore {
			bestScore = s
			best = e
		}
	}
	return best, bestScore, bestScore >= 0, nil
}

// evictEvery controls how often (by insert count) lazy eviction runs.
const evictEvery = 100

// Put inserts an entry using an O(1) ring buffer. Every evictEvery inserts,
// expired entries are evicted to reclaim memory from the slice.
func (m *MemoryStore) Put(_ context.Context, e Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Lazy eviction: sweep expired entries periodically to reclaim memory.
	if m.count > 0 && m.count%evictEvery == 0 {
		m.evict(time.Now())
	}
	if m.max > 0 && m.count >= m.max {
		m.entries[m.cursor] = e // overwrite oldest (O(1))
	} else {
		m.entries = append(m.entries, e)
		m.count++
	}
	if m.max > 0 {
		m.cursor = (m.cursor + 1) % m.max
	}
	return nil
}

// Len reports the entry count.
func (m *MemoryStore) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.count
}

// cosine returns the cosine similarity of two equal-length vectors in [-1, 1].
// Mismatched or zero-magnitude vectors return 0.
func cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
