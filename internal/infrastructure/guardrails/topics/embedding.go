package topics

import (
	"context"
	"math"
	"strings"
	"sync"
)

// topicVectorCache memoizes embeddings for topic strings. The cache is
// unbounded but topic lists are short (10s) so we don't bother with
// eviction; pruning lives in [vectorCacheBudget] for safety.
type topicVectorCache struct {
	mu   sync.RWMutex
	data map[string][]float32
}

const vectorCacheBudget = 1024

func newTopicVectorCache() *topicVectorCache {
	return &topicVectorCache{data: make(map[string][]float32)}
}

// get returns the cached embedding for topic, computing it on the first
// call via emb. Concurrent calls for the same topic may both compute, which
// is harmless — last-writer wins.
func (c *topicVectorCache) get(ctx context.Context, emb Embedder, topic string) ([]float32, error) {
	key := normalizeTopic(topic)
	c.mu.RLock()
	v, ok := c.data[key]
	c.mu.RUnlock()
	if ok {
		return v, nil
	}
	vec, err := emb.Embed(ctx, key)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if len(c.data) >= vectorCacheBudget {
		// Drop the cache wholesale rather than implementing an LRU; topic
		// lists rarely change, so cold rebuilds are cheap and infrequent.
		c.data = make(map[string][]float32, vectorCacheBudget)
	}
	c.data[key] = vec
	c.mu.Unlock()
	return vec, nil
}

func normalizeTopic(topic string) string {
	return strings.ToLower(strings.TrimSpace(topic))
}

// cosine returns the cosine similarity of two vectors in [-1, 1]. If either
// vector is zero or the lengths differ, it returns 0.
func cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		x := float64(a[i])
		y := float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
