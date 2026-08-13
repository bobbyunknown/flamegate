package wasm

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
)

// Sentinel errors for lifecycle transitions.
var (
	ErrInvalidTransition = errors.New("wasm: invalid state transition")
)

// stateNames maps state constants to human-readable names.
var stateNames = map[int32]string{
	StatePending:  "PENDING",
	StateActive:   "ACTIVE",
	StateDisabled: "DISABLED",
	StateError:    "ERROR",
	StateUpdating: "UPDATING",
}

// validTransitions defines which state transitions are allowed.
var validTransitions = map[int32][]int32{
	StatePending:  {StateActive},
	StateActive:   {StateDisabled, StateError, StateUpdating},
	StateDisabled: {StateActive},
	StateError:    {StateActive},
	StateUpdating: {StateActive},
}

// CanTransition reports whether a transition from → to is valid.
func CanTransition(from, to int32) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, t := range targets {
		if t == to {
			return true
		}
	}
	return false
}

// Transition validates and applies a state transition on the Module.
// Returns ErrInvalidTransition if the transition is not allowed.
func Transition(mod *Module, to int32, log *logrus.Entry) error {
	from := mod.State()
	if !CanTransition(from, to) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, stateName(from), stateName(to))
	}

	mod.SetState(to)

	if log != nil {
		log.Infof("extension %s: %s → %s", mod.slug, stateName(from), stateName(to))
	}
	return nil
}

// TransitionToError transitions the module to ERROR state and records the error.
func TransitionToError(mod *Module, log *logrus.Entry, lastErr string) error {
	if err := Transition(mod, StateError, log); err != nil {
		return err
	}
	mod.SetLastError(lastErr)
	if log != nil {
		log.WithField("error", lastErr).Warn("extension entered ERROR state")
	}
	return nil
}

// TransitionToUpdating transitions the module to UPDATING state (reinstall start).
func TransitionToUpdating(mod *Module, log *logrus.Entry) error {
	return Transition(mod, StateUpdating, log)
}

// TransitionToActive transitions the module back to ACTIVE.
// For UPDATING → ACTIVE: lastErr distinguishes upgrade (empty) from rollback (non-empty).
func TransitionToActive(mod *Module, log *logrus.Entry, lastErr string) error {
	if err := Transition(mod, StateActive, log); err != nil {
		return err
	}
	mod.SetLastError(lastErr)
	if log != nil && lastErr != "" {
		log.WithField("last_error", lastErr).Warn("extension rolled back to ACTIVE (compile failed)")
	}
	return nil
}

// Reinstall performs a compile-then-swap upgrade of a WASM module.
// Steps:
// 1. Transition to UPDATING
// 2. Compile new module
// 3. If compile fails → rollback to ACTIVE with last_error, return error
// 4. If compile succeeds → swap compiled reference
// 5. Drain in-flight requests (caller handles WaitGroup)
// 6. Transition to ACTIVE (last_error empty = upgrade success)
//
// The caller MUST drain in-flight requests and close the old module after
// this function returns successfully.
func Reinstall(engine *Engine, mod *Module, slug string, wasmBytes []byte, extCfg ExtensionConfig, log *logrus.Entry) error {
	// Step 1: UPDATING
	if err := TransitionToUpdating(mod, log); err != nil {
		return fmt.Errorf("wasm: reinstall %s: %w", slug, err)
	}

	// Step 2: compile new module
	if err := engine.Compile(context.Background(), slug, wasmBytes, extCfg); err != nil {
		// Step 3: compile failed → rollback
		_ = TransitionToActive(mod, log, err.Error())
		return fmt.Errorf("wasm: reinstall %s compile failed: %w", slug, err)
	}

	// Step 4: compile succeeded → swap reference (engine.Compile replaces in map)

	// Step 5-6: caller drains in-flight, then transitions to ACTIVE
	return TransitionToActive(mod, log, "")
}

// stateName returns the human-readable name for a state constant.
func stateName(s int32) string {
	if name, ok := stateNames[s]; ok {
		return name
	}
	return fmt.Sprintf("UNKNOWN(%d)", s)
}
