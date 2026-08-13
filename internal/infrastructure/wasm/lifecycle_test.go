package wasm

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func logEntry() *logrus.Entry {
	return logrus.WithField("test", true)
}

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from, to int32
		valid    bool
	}{
		{StatePending, StateActive, true},
		{StatePending, StateDisabled, false},
		{StateActive, StateDisabled, true},
		{StateActive, StateError, true},
		{StateActive, StateUpdating, true},
		{StateActive, StatePending, false},
		{StateDisabled, StateActive, true},
		{StateDisabled, StateError, false},
		{StateError, StateActive, true},
		{StateError, StateDisabled, false},
		{StateUpdating, StateActive, true},
		{StateUpdating, StateDisabled, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.valid, CanTransition(tt.from, tt.to),
			"%s → %s", stateName(tt.from), stateName(tt.to))
	}
}

func TestTransition_Valid(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	mod := NewModule(e, "test-ext", 4)
	assert.Equal(t, StateActive, mod.State())

	assert.NoError(t, Transition(mod, StateDisabled, logEntry()))
	assert.Equal(t, StateDisabled, mod.State())

	assert.NoError(t, Transition(mod, StateActive, logEntry()))
	assert.Equal(t, StateActive, mod.State())
}

func TestTransition_Invalid(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	mod := NewModule(e, "test-ext", 4)
	assert.Equal(t, StateActive, mod.State())

	err := Transition(mod, StatePending, logEntry())
	assert.ErrorIs(t, err, ErrInvalidTransition)
	assert.Equal(t, StateActive, mod.State(), "state should not change on invalid transition")
}

func TestTransition_FullCycle(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	mod := NewModule(e, "test-ext", 4)

	transitions := []struct {
		to   int32
		name string
	}{
		{StateDisabled, "DISABLED"},
		{StateActive, "ACTIVE"},
		{StateError, "ERROR"},
		{StateActive, "ACTIVE"},
		{StateUpdating, "UPDATING"},
		{StateActive, "ACTIVE"},
	}

	for _, tr := range transitions {
		assert.NoError(t, Transition(mod, tr.to, logEntry()))
		assert.Equal(t, tr.to, mod.State())
	}
}

func TestTransitionToError(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	mod := NewModule(e, "test-ext", 4)
	assert.NoError(t, TransitionToError(mod, logEntry(), "panic: nil pointer"))
	assert.Equal(t, StateError, mod.State())
	assert.Equal(t, "panic: nil pointer", mod.LastError())
}

func TestTransitionToActive_Rollback(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	mod := NewModule(e, "test-ext", 4)
	mod.SetState(StateUpdating)

	assert.NoError(t, TransitionToActive(mod, logEntry(), "compile failed: bad wasm"))
	assert.Equal(t, StateActive, mod.State())
	assert.Equal(t, "compile failed: bad wasm", mod.LastError())
}

func TestTransitionToActive_Upgrade(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	mod := NewModule(e, "test-ext", 4)
	mod.SetState(StateUpdating)

	assert.NoError(t, TransitionToActive(mod, logEntry(), ""))
	assert.Equal(t, StateActive, mod.State())
	assert.Equal(t, "", mod.LastError())
}

func TestStateName(t *testing.T) {
	assert.Equal(t, "ACTIVE", stateName(StateActive))
	assert.Equal(t, "DISABLED", stateName(StateDisabled))
	assert.Equal(t, "ERROR", stateName(StateError))
	assert.Equal(t, "UPDATING", stateName(StateUpdating))
	assert.Equal(t, "PENDING", stateName(StatePending))
	assert.Equal(t, "UNKNOWN(99)", stateName(99))
}

func TestReinstall_Success(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	// Pre-compile so module exists for Reinstall to update.
	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	mod := NewModule(e, "test-ext", 4)
	assert.Equal(t, StateActive, mod.State())

	// Reinstall with valid WASM should succeed: ACTIVE → UPDATING → ACTIVE.
	err := Reinstall(e, mod, "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}, logEntry())
	assert.NoError(t, err)
	assert.Equal(t, StateActive, mod.State())
	assert.Equal(t, "", mod.LastError())
}

func TestReinstall_CompileFailure(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	// Pre-compile so module exists.
	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	mod := NewModule(e, "test-ext", 4)
	assert.Equal(t, StateActive, mod.State())

	// Reinstall with invalid WASM should rollback: ACTIVE → UPDATING → ACTIVE (with error).
	err := Reinstall(e, mod, "test-ext", []byte{0x00, 0x01, 0x02}, ExtensionConfig{
		Slug: "test-ext",
	}, logEntry())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compile failed")
	assert.Equal(t, StateActive, mod.State(), "should rollback to ACTIVE on compile failure")
	assert.NotEmpty(t, mod.LastError(), "should record compile error")
}

func TestReinstall_InvalidTransition(t *testing.T) {
	e := newEngineForTest(t)
	defer e.Close() //nolint:errcheck // best-effort close

	require.NoError(t, e.Compile(context.Background(), "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}))

	mod := NewModule(e, "test-ext", 4)
	mod.SetState(StateDisabled)

	// Reinstall from DISABLED should fail (DISABLED → UPDATING is invalid).
	err := Reinstall(e, mod, "test-ext", compileMinimalWASM(t), ExtensionConfig{
		Slug: "test-ext",
	}, logEntry())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTransition)
}
