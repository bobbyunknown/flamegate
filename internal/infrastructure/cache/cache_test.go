package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

func resp(text string) *core.ChatResponse {
	return &core.ChatResponse{
		Model:   "gpt-4o",
		Message: core.Message{Role: core.RoleAssistant, Content: []core.ContentPart{{Type: core.PartText, Text: text}}},
	}
}

func TestCache_Disabled(t *testing.T) {
	c := New(Config{Enabled: false}, nil)
	require.NoError(t, c.Store(context.Background(), []float32{1, 0}, resp("hi")))
	_, ok, err := c.Lookup(context.Background(), []float32{1, 0})
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCache_HitOnIdenticalVector(t *testing.T) {
	c := New(Config{Enabled: true, SimilarityThreshold: 0.95, TTL: time.Hour}, nil)
	vec := []float32{0.1, 0.9, 0.3}
	require.NoError(t, c.Store(context.Background(), vec, resp("cached answer")))

	got, ok, err := c.Lookup(context.Background(), vec)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "cached answer", got.Message.TextContent())
}

func TestCache_MissBelowThreshold(t *testing.T) {
	c := New(Config{Enabled: true, SimilarityThreshold: 0.99, TTL: time.Hour}, nil)
	require.NoError(t, c.Store(context.Background(), []float32{1, 0, 0}, resp("x")))

	// Orthogonal vector -> cosine 0, well below threshold.
	_, ok, err := c.Lookup(context.Background(), []float32{0, 1, 0})
	require.NoError(t, err)
	require.False(t, ok)
}

func TestCache_NearMatchHits(t *testing.T) {
	c := New(Config{Enabled: true, SimilarityThreshold: 0.95, TTL: time.Hour}, nil)
	require.NoError(t, c.Store(context.Background(), []float32{1, 0, 0}, resp("near")))

	// Slightly perturbed but highly similar vector.
	got, ok, err := c.Lookup(context.Background(), []float32{0.99, 0.05, 0.02})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "near", got.Message.TextContent())
}

func TestCache_TTLExpiry(t *testing.T) {
	c := New(Config{Enabled: true, SimilarityThreshold: 0.9, TTL: time.Millisecond}, nil)
	vec := []float32{1, 1}
	require.NoError(t, c.Store(context.Background(), vec, resp("stale")))
	time.Sleep(5 * time.Millisecond)

	_, ok, err := c.Lookup(context.Background(), vec)
	require.NoError(t, err)
	require.False(t, ok, "expired entry must be a miss")
}

func TestMemoryStore_Eviction(t *testing.T) {
	store := NewMemoryStore(2, 0)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, store.Put(ctx, Entry{Vector: []float32{float32(i)}, StoredAt: time.Now().Add(time.Duration(i) * time.Millisecond)}))
	}
	require.LessOrEqual(t, store.Len(), 2, "store must respect max capacity")
}

func TestCosine(t *testing.T) {
	require.InDelta(t, 1.0, cosine([]float32{1, 2, 3}, []float32{1, 2, 3}), 1e-9)
	require.InDelta(t, 0.0, cosine([]float32{1, 0}, []float32{0, 1}), 1e-9)
	require.Equal(t, 0.0, cosine([]float32{1, 0}, []float32{1, 0, 0}), "length mismatch -> 0")
	require.Equal(t, 0.0, cosine(nil, nil))
}
