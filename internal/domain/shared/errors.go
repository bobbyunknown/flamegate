package shared

import (
	"errors"
	"fmt"
	"time"
)

// Domain sentinel errors — the canonical set for domain-layer use-cases.
//
// These wrap the matching HTTP semantics and are checked with errors.Is in the
// HTTP error mapper.
//
// infrastructure/persistence/schema.ErrNotFound is kept as a separate value
// wrapping gorm.ErrRecordNotFound; callers at the infrastructure boundary may
// continue to use it directly. Domain code should prefer these sentinels and
// wrap infrastructure errors via errors.Is / %w so that callers can match
// either layer uniformly.
var (
	// ErrNotFound indicates a requested entity does not exist.
	ErrNotFound = errors.New("not found")

	// ErrConflict indicates an operation conflicts with current state
	// (e.g. duplicate key, concurrent modification).
	ErrConflict = errors.New("conflict")

	// ErrValidation indicates the input failed a domain invariant check.
	ErrValidation = errors.New("validation failed")

	// ErrUnauthorized indicates missing or invalid credentials.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the caller lacks permission for the operation.
	ErrForbidden = errors.New("forbidden")
)

// ErrorKind classifies an upstream failure so the dispatcher can decide whether
// to retry the same account, fall back to the next account/model, or surface
// the error to the client immediately.
type ErrorKind string

const (
	// ErrAuth: credential rejected/expired. Try refresh, then next account.
	ErrAuth ErrorKind = "auth"
	// ErrRateLimit: quota/rate exceeded. Cool down this account, fall back.
	ErrRateLimit ErrorKind = "rate_limit"
	// ErrQuotaExhausted: subscription/budget fully consumed. Fall back, longer cooldown.
	ErrQuotaExhausted ErrorKind = "quota_exhausted"
	// ErrUpstream: 5xx / transient upstream fault. Retry then fall back.
	ErrUpstream ErrorKind = "upstream"
	// ErrTimeout: stream stalled or request timed out. Retry then fall back.
	ErrTimeout ErrorKind = "timeout"
	// ErrBadRequest: 4xx caused by the request itself. Do NOT fall back; surface.
	ErrBadRequest ErrorKind = "bad_request"
	// ErrCapability: chosen model lacks a required capability. Skip, no surface.
	ErrCapability ErrorKind = "capability"
	// ErrBudgetBlocked: FlameGate budget guard rejected before dispatch.
	ErrBudgetBlocked ErrorKind = "budget_blocked"
	// ErrPolicyBlocked: a guardrail policy rejected the request before dispatch.
	// Not fallbackable — falling back to another provider would not change the
	// outcome of a content-safety decision.
	ErrPolicyBlocked ErrorKind = "policy_blocked"
	// ErrInternal: router-internal fault.
	ErrInternal ErrorKind = "internal"
)

// ProviderError is the structured error connectors and the pipeline return. It
// carries enough context for the dispatcher to make a fallback decision and for
// the gateway to render an accurate HTTP status to the client.
type ProviderError struct {
	Kind ErrorKind
	// Provider and Model identify where the failure originated.
	Provider string
	Model    string
	// StatusCode is the upstream HTTP status, if any.
	StatusCode int
	// Message is a human-readable summary safe to log.
	Message string
	// RetryAfter, when non-zero, is the upstream-suggested cooldown.
	RetryAfter time.Duration
	// Cause is the wrapped underlying error.
	Cause error
}

func (e *ProviderError) Error() string {
	if e.Provider != "" {
		return fmt.Sprintf("%s: %s (provider=%s model=%s status=%d)",
			e.Kind, e.Message, e.Provider, e.Model, e.StatusCode)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *ProviderError) Unwrap() error { return e.Cause }

// Retryable reports whether trying the same model again (possibly on another
// account) could succeed.
func (e *ProviderError) Retryable() bool {
	switch e.Kind {
	case ErrUpstream, ErrTimeout, ErrRateLimit:
		return true
	default:
		return false
	}
}

// Fallbackable reports whether the dispatcher should advance to the next
// candidate in the chain rather than surfacing this error to the client.
func (e *ProviderError) Fallbackable() bool {
	switch e.Kind {
	case ErrBadRequest, ErrBudgetBlocked, ErrPolicyBlocked:
		return false
	default:
		return true
	}
}

// AsProviderError extracts a *ProviderError from err, or wraps it as ErrInternal.
func AsProviderError(err error) *ProviderError {
	if err == nil {
		return nil
	}
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe
	}
	return &ProviderError{Kind: ErrInternal, Message: err.Error(), Cause: err}
}

// NewProviderError constructs a ProviderError with the given kind and message.
func NewProviderError(kind ErrorKind, msg string) *ProviderError {
	return &ProviderError{Kind: kind, Message: msg}
}
