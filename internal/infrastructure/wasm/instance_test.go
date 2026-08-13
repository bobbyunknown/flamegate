package wasm

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModule_AcquireRelease(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug:    "test-ext",
		Timeout: 60 * time.Second,
	}))

	mod := NewModule(e, "test-ext", 4)
	assert.Equal(t, StateActive, mod.State())
	assert.Equal(t, int64(0), mod.ActiveCount())

	env := &InvokeEnv{Ctx: context.Background(), Slug: "test-ext"}
	inst, err := mod.Acquire(context.Background(), env)
	require.NoError(t, err)
	assert.Equal(t, int64(1), mod.ActiveCount())

	mod.Release(inst)
	assert.Equal(t, int64(0), mod.ActiveCount())
}

func TestModule_AcquireNotActive(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	mod := NewModule(e, "test-ext", 4)
	mod.SetState(StateDisabled)

	env := &InvokeEnv{Ctx: context.Background(), Slug: "test-ext"}
	_, err := mod.Acquire(context.Background(), env)
	assert.ErrorIs(t, err, ErrModuleNotActive)
}

func TestModule_PoolExhausted(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	mod := NewModule(e, "test-ext", 1)

	env := &InvokeEnv{Ctx: context.Background(), Slug: "test-ext"}
	inst, err := mod.Acquire(context.Background(), env)
	require.NoError(t, err)

	_, err = mod.Acquire(context.Background(), env)
	assert.ErrorIs(t, err, ErrPoolExhausted)

	mod.Release(inst)

	inst2, err := mod.Acquire(context.Background(), env)
	require.NoError(t, err)
	mod.Release(inst2)
}

func TestModule_ConcurrentAcquireRelease(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	const maxInst = 8
	mod := NewModule(e, "test-ext", maxInst)

	var wg sync.WaitGroup
	var successes atomic.Int64

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for {
				env := &InvokeEnv{Ctx: ctx, Slug: "test-ext"}
				inst, err := mod.Acquire(ctx, env)
				if errors.Is(err, ErrPoolExhausted) {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				if err != nil {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				successes.Add(1)
				mod.Release(inst)
				return
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(20), successes.Load())
	assert.Equal(t, int64(0), mod.ActiveCount())
}

func TestModule_StateTransitions(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	mod := NewModule(e, "test-ext", 4)

	mod.SetState(StateDisabled)
	assert.Equal(t, StateDisabled, mod.State())

	mod.SetState(StateActive)
	assert.Equal(t, StateActive, mod.State())

	mod.SetState(StateError)
	assert.Equal(t, StateError, mod.State())

	mod.SetState(StateActive)
	assert.Equal(t, StateActive, mod.State())

	mod.SetState(StateUpdating)
	assert.Equal(t, StateUpdating, mod.State())

	mod.SetState(StateActive)
	assert.Equal(t, StateActive, mod.State())
}
