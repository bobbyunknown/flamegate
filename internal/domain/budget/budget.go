// Package budget defines the Budget aggregate.
package budget

import (
	"errors"
	"time"
)

var (
	ErrEmptyScope = errors.New("budget: scope must not be empty")
	ErrExceeded   = errors.New("budget: limit exceeded")
)

// Scope identifies what a budget applies to.
type Scope string

const (
	ScopeTenant  Scope = "tenant"
	ScopeProject Scope = "project"
	ScopeAPIKey  Scope = "api_key"
)

// Period defines the budget reset period.
type Period string

const (
	PeriodDaily   Period = "daily"
	PeriodMonthly Period = "monthly"
	PeriodTotal   Period = "total"
)

// Budget enforces a spend and/or token limit over a period.
type Budget struct {
	ID          string
	TenantID    string
	ScopeKind   Scope
	ScopeID     string
	LimitMicros int64
	LimitTokens int64
	Period      Period
	AlertPct    int
	HardCutoff  bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// NewBudget constructs a Budget with required fields validated.
func NewBudget(id, tenantID string, scope Scope, scopeID string, now time.Time) (*Budget, error) {
	if scope == "" {
		return nil, ErrEmptyScope
	}
	return &Budget{
		ID:        id,
		TenantID:  tenantID,
		ScopeKind: scope,
		ScopeID:   scopeID,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// IsExceeded reports whether the given usage exceeds this budget's limits.
func (b *Budget) IsExceeded(costMicros int64, tokens int64) bool {
	if b.LimitMicros > 0 && costMicros >= b.LimitMicros {
		return true
	}
	if b.LimitTokens > 0 && tokens >= b.LimitTokens {
		return true
	}
	return false
}

// RemainingMicros returns remaining cost budget. Returns -1 if no limit.
func (b *Budget) RemainingMicros(spentMicros int64) int64 {
	if b.LimitMicros <= 0 {
		return -1
	}
	r := b.LimitMicros - spentMicros
	if r < 0 {
		return 0
	}
	return r
}
