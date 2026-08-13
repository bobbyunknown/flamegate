package wasm

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/tetratelabs/wazero/api"
)

// Module lifecycle states.
const (
	StatePending  int32 = iota // compiled, not yet validated
	StateActive                // ready to serve requests
	StateDisabled              // admin-disabled
	StateError                 // panic threshold exceeded
	StateUpdating              // reinstall in progress
)

// Sentinel errors for instance lifecycle.
var (
	ErrModuleNotActive = errors.New("wasm: module not active")
	ErrPoolExhausted   = errors.New("wasm: max concurrent instances reached")
)

// Module wraps a compiled WASM module with lifecycle state and concurrency control.
// It does NOT pool instances — each Acquire creates a fresh instance (no-reuse
// for credential isolation). The atomic active counter limits concurrency.
type Module struct {
	engine  *Engine
	slug    string
	state   atomic.Int32
	active  atomic.Int64
	maxInst int64
	lastErr string
}

// NewModule creates a Module in Active state.
func NewModule(engine *Engine, slug string, maxInst int) *Module {
	m := &Module{
		engine:  engine,
		slug:    slug,
		maxInst: int64(maxInst),
	}
	m.state.Store(StateActive)
	return m
}

// State returns the current lifecycle state.
func (m *Module) State() int32 { return m.state.Load() }

// SetState transitions the module to a new state.
func (m *Module) SetState(s int32) { m.state.Store(s) }

// ActiveCount returns the number of currently active instances.
func (m *Module) ActiveCount() int64 { return m.active.Load() }

// LastError returns the last error recorded during lifecycle transitions.
func (m *Module) LastError() string { return m.lastErr }

// SetLastError records an error on the module for later retrieval.
func (m *Module) SetLastError(err string) { m.lastErr = err }

// Acquire creates a fresh WASM module instance with the given host function env.
// Returns ErrModuleNotActive if state != Active, ErrPoolExhausted if at max concurrency.
// Caller MUST call Release() when done.
func (m *Module) Acquire(ctx context.Context, env *InvokeEnv) (api.Module, error) {
	if m.state.Load() != StateActive {
		return nil, ErrModuleNotActive
	}
	if m.active.Load() >= m.maxInst {
		return nil, ErrPoolExhausted
	}
	m.active.Add(1)

	inst, err := m.engine.Instantiate(ctx, m.slug, env)
	if err != nil {
		m.active.Add(-1)
		return nil, fmt.Errorf("wasm: acquire %s: %w", m.slug, err)
	}
	return inst, nil
}

// Release closes the instance and decrements the active counter.
func (m *Module) Release(inst api.Module) {
	if inst != nil {
		_ = inst.Close(context.Background())
	}
	m.active.Add(-1)
}
