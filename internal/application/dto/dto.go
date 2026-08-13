// Package dto defines Data Transfer Objects for the application layer.
// These are transport types — NOT domain entities. Domain entities live in
// internal/core/ and carry business behavior. DTOs are plain structs for
// marshaling/unmarshaling between HTTP handlers and usecases.
//
// Fields use JSON tags for HTTP serialization and may differ from the
// domain representation (e.g. omitting sensitive fields like hashes).
package dto

import "time"

// ---------------------------------------------------------------------------
// Key DTOs
// ---------------------------------------------------------------------------

// CreateKeyRequest is the input for creating a new API key.
type CreateKeyRequest struct {
	TenantID    string   `json:"-"`
	Name        string   `json:"name"`
	PlanID      *string  `json:"plan_id,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// CreateKeyResult is returned after creating a key. Plaintext is shown once.
type CreateKeyResult struct {
	ID        string `json:"id"`
	Plaintext string `json:"plaintext"`
	Name      string `json:"name"`
}

// UpdateKeyRequest is the input for updating a key's metadata.
type UpdateKeyRequest struct {
	ID          string    `json:"id"`
	Name        *string   `json:"name,omitempty"`
	PlanID      *string   `json:"plan_id,omitempty"`
	Permissions *[]string `json:"permissions,omitempty"`
}

// ---------------------------------------------------------------------------
// Account DTOs
// ---------------------------------------------------------------------------

// CreateAccountRequest is the input for registering a provider account.
type CreateAccountRequest struct {
	TenantID string  `json:"-"`
	Provider string  `json:"provider"`
	Name     string  `json:"name"`
	AuthKind string  `json:"auth_kind"`
	APIKey   string  `json:"api_key,omitempty"`
	BaseURL  *string `json:"base_url,omitempty"`
	Region   *string `json:"region,omitempty"`
	Priority int     `json:"priority"`
}

// UpdateAccountRequest is the input for modifying a provider account.
type UpdateAccountRequest struct {
	Name     *string `json:"name,omitempty"`
	Priority *int    `json:"priority,omitempty"`
	Disabled *bool   `json:"disabled,omitempty"`
}

// TestAccountResult is returned after testing an account connection.
type TestAccountResult struct {
	Success bool          `json:"success"`
	Latency time.Duration `json:"latency"`
	Error   string        `json:"error,omitempty"`
	Models  []string      `json:"models,omitempty"`
}

// ---------------------------------------------------------------------------
// Plan DTOs
// ---------------------------------------------------------------------------

// CreatePlanRequest is the input for creating a new plan.
type CreatePlanRequest struct {
	TenantID         string   `json:"-"`
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	LimitMicros      *int64   `json:"limit_micros,omitempty"`
	LimitTokens      *int64   `json:"limit_tokens,omitempty"`
	RPMLimit         *int     `json:"rpm_limit,omitempty"`
	TPMLimit         *int     `json:"tpm_limit,omitempty"`
	ConcurrencyLimit *int     `json:"concurrency_limit,omitempty"`
	Period           string   `json:"period,omitempty"`
	AlertPct         *int     `json:"alert_pct,omitempty"`
	HardCutoff       bool     `json:"hard_cutoff,omitempty"`
	AllowedModels    []string `json:"allowed_models,omitempty"`
}

// UpdatePlanRequest is the input for modifying a plan.
type UpdatePlanRequest struct {
	ID               string    `json:"id"`
	Name             *string   `json:"name,omitempty"`
	Description      *string   `json:"description,omitempty"`
	LimitMicros      *int64    `json:"limit_micros,omitempty"`
	LimitTokens      *int64    `json:"limit_tokens,omitempty"`
	RPMLimit         *int      `json:"rpm_limit,omitempty"`
	TPMLimit         *int      `json:"tpm_limit,omitempty"`
	ConcurrencyLimit *int      `json:"concurrency_limit,omitempty"`
	Period           *string   `json:"period,omitempty"`
	AlertPct         *int      `json:"alert_pct,omitempty"`
	HardCutoff       *bool     `json:"hard_cutoff,omitempty"`
	AllowedModels    *[]string `json:"allowed_models,omitempty"`
}
