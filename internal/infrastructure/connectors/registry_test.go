package connectors

import (
	"context"
	"testing"

	core "github.com/bobbyunknown/flamegate/internal/domain/provider"
	"github.com/bobbyunknown/flamegate/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubConnector is a minimal Connector for testing.
type stubConnector struct{ id string }

func (s stubConnector) ID() string                { return s.id }
func (s stubConnector) Dialect() shared.Dialect { return shared.DialectOpenAI }
func (s stubConnector) Chat(ctx context.Context, req *shared.ChatRequest, creds core.Credentials) (*shared.ChatResponse, error) {
	return nil, nil
}
func (s stubConnector) Stream(ctx context.Context, req *shared.ChatRequest, creds core.Credentials, cfg core.StreamConfig) (<-chan shared.StreamChunk, error) {
	return nil, nil
}

func TestRegistryGet_Tier1NativeOnly(t *testing.T) {
	reg := NewRegistry( stubConnector{"openai"}, stubConnector{"anthropic"}, stubConnector{"gemini"} )
	reg.SetWASMFallback(func(id string) (core.Connector, bool) { return nil, false })

	// Tier-1 provider: native even without WASM
	c, err := reg.Get("openai")
	require.NoError(t, err)
	assert.Equal(t, "openai", c.ID())

	c, err = reg.Get("anthropic")
	require.NoError(t, err)
	assert.Equal(t, "anthropic", c.ID())
}


func TestRegistryGet_NonTier1WASMFallback(t *testing.T) {
	wasm := stubConnector{"kiro-wasm"}

	reg := NewRegistry() // No native connector
	reg.SetWASMFallback(func(id string) (core.Connector, bool) {
		if id == "kiro" {
			return wasm, true
		}
		return nil, false
	})

	// Non-Tier-1 without native but with WASM → WASM wins
	c, err := reg.Get("kiro")
	require.NoError(t, err)
	assert.Equal(t, "kiro-wasm", c.ID(), "non-Tier-1 should use WASM when native absent")
}

func TestRegistryGet_UnknownNoWASM(t *testing.T) {
	reg := NewRegistry()
	reg.SetWASMFallback(func(id string) (core.Connector, bool) { return nil, false })

	_, err := reg.Get("xiaomi-mimo")
	require.Error(t, err)
}

func TestRegistryGet_ForceWasmAll(t *testing.T) {
	reg := NewRegistry(stubConnector{"openai"})
	wasm := stubConnector{"openai-wasm"}
	reg.SetWASMFallback(func(id string) (core.Connector, bool) {
		if id == "openai" {
			return wasm, true
		}
		return nil, false
	})

	// Without force_wasm_all: Tier-1 native
	c, err := reg.Get("openai")
	require.NoError(t, err)
	assert.Equal(t, "openai", c.ID(), "Tier-1 stays native without force_wasm_all")

	// With force_wasm_all: Tier-1 WASM
	reg.SetForceWasmAll(true)
	c, err = reg.Get("openai")
	require.NoError(t, err)
	assert.Equal(t, "openai-wasm", c.ID(), "Tier-1 uses WASM when force_wasm_all enabled")
}
