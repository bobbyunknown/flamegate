// Package account defines the provider Account aggregate.
package account

import (
	"errors"
	"time"
)

var (
	ErrEmptyProvider = errors.New("account: provider must not be empty")
	ErrSuspended     = errors.New("account: cannot cooldown a suspended account")
	ErrCooldownPast  = errors.New("account: cooldown must be in the future")
)

// Status represents the account's operational state.
type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusCooldown  Status = "cooldown"
)

// Account holds an upstream provider credential.
type Account struct {
	ID             string
	TenantID       string
	Provider       string
	Label          string
	AuthKind       string
	Status         Status
	CooldownUntil  *time.Time
	BackoffLevel   int
	Disabled       bool
	NeedsReconnect bool
	ProxyPoolID    string
	Priority       int
	Metadata       string // JSON blob
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// NewAccount constructs an Account with required fields.
func NewAccount(id, tenantID, provider, label, authKind string, now time.Time) (*Account, error) {
	if provider == "" {
		return nil, ErrEmptyProvider
	}
	return &Account{
		ID:        id,
		TenantID:  tenantID,
		Provider:  provider,
		Label:     label,
		AuthKind:  authKind,
		Status:    StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Suspend marks the account as suspended.
func (a *Account) Suspend(now time.Time) error {
	a.Status = StatusSuspended
	a.UpdatedAt = now
	return nil
}

// Activate restores the account to active status and clears cooldown.
func (a *Account) Activate(now time.Time) error {
	a.Status = StatusActive
	a.CooldownUntil = nil
	a.BackoffLevel = 0
	a.UpdatedAt = now
	return nil
}

// SetCooldown marks the account as cooling down until the given time.
func (a *Account) SetCooldown(until time.Time, now time.Time) error {
	if a.Status == StatusSuspended {
		return ErrSuspended
	}
	if !until.After(now) {
		return ErrCooldownPast
	}
	a.Status = StatusCooldown
	a.CooldownUntil = &until
	a.UpdatedAt = now
	return nil
}

// IsOnCooldown reports whether the account is in an active cooldown period.
func (a *Account) IsOnCooldown(now time.Time) bool {
	if a.Status != StatusCooldown || a.CooldownUntil == nil {
		return false
	}
	return now.Before(*a.CooldownUntil)
}

// IsAvailable reports whether the account can serve requests.
func (a *Account) IsAvailable(now time.Time) bool {
	if a.Disabled || a.NeedsReconnect {
		return false
	}
	if a.Status == StatusSuspended {
		return false
	}
	if a.IsOnCooldown(now) {
		return false
	}
	return true
}
