package extstore

import (
	"sync"
	"time"
)

// cacheEntry holds a cached value and its expiry time.
type cacheEntry struct {
	val any
	exp time.Time
}

// TTLCache is a small concurrency-safe in-memory TTL cache with lazy expiry.
type TTLCache struct {
	mu sync.RWMutex
	m  map[string]cacheEntry
}

// NewTTLCache returns an empty TTLCache.
func NewTTLCache() *TTLCache {
	return &TTLCache{m: make(map[string]cacheEntry)}
}

// Set stores val under key for the given ttl. A ttl <= 0 evicts immediately.
func (c *TTLCache) Set(key string, val any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ttl <= 0 {
		delete(c.m, key)
		return
	}
	c.m[key] = cacheEntry{val: val, exp: time.Now().Add(ttl)}
}

// Get returns the value if present and not expired.
func (c *TTLCache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(e.exp) {
		c.mu.Lock()
		delete(c.m, key)
		c.mu.Unlock()
		return nil, false
	}
	return e.val, true
}

// Delete removes a key.
func (c *TTLCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, key)
}