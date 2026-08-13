package connectors

import (
	"context"
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/domain/provider"
	"github.com/bobbyunknown/flamegate/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConnector implements core.Connector for testing.
type mockConnector struct{ id string }

func (m *mockConnector) ID() string                   { return m.id }
func (m *mockConnector) Dialect() shared.Dialect       { return shared.DialectOpenAI }
func (m *mockConnector) Chat(_ context.Context, _ *shared.ChatRequest, _ provider.Credentials) (*shared.ChatResponse, error) {
	return nil, nil
}
func (m *mockConnector) Stream(_ context.Context, _ *shared.ChatRequest, _ provider.Credentials, _ provider.StreamConfig) (<-chan shared.StreamChunk, error) {
	return nil, nil
}

func TestRegistry_SetWASMFallback_NativeFirst(t *testing.T) {
	r := DefaultRegistry()

	wasmCalled := false
	r.SetWASMFallback(func(slug string) (core.Connector, bool) {
		wasmCalled = true
		return nil, false
	})

	c, err := r.Get("openai")
	require.NoError(t, err)
	assert.Equal(t, "openai", c.ID())
	assert.False(t, wasmCalled, "WASM fallback should not be called for native providers")
}

func TestRegistry_SetWASMFallback_WASMHit(t *testing.T) {
	r := NewRegistry()

	r.SetWASMFallback(func(slug string) (core.Connector, bool) {
		if slug == "my-ext" {
			return &mockConnector{id: "my-ext"}, true
		}
		return nil, false
	})

	c, err := r.Get("my-ext")
	require.NoError(t, err)
	assert.Equal(t, "my-ext", c.ID())
}

func TestRegistry_SetWASMFallback_WASMMiss(t *testing.T) {
	r := NewRegistry()

	r.SetWASMFallback(func(slug string) (core.Connector, bool) {
		return nil, false
	})

	_, err := r.Get("nonexistent")
	assert.Error(t, err)
}

func TestRegistry_Has_WithWASM(t *testing.T) {
	r := NewRegistry()

	r.SetWASMFallback(func(slug string) (core.Connector, bool) {
		if slug == "wasm-ext" {
			return &mockConnector{id: "wasm-ext"}, true
		}
		return nil, false
	})

	assert.True(t, r.Has("wasm-ext"))
	assert.False(t, r.Has("nonexistent"))
}

func TestDefaultRegistry_Providers(t *testing.T) {
	r := DefaultRegistry()
	providers := r.Providers()
	assert.NotEmpty(t, providers)
	found := false
	for _, p := range providers {
		if p == "openai" {
			found = true
			break
		}
	}
	assert.True(t, found, "openai should be in providers list")
}
