// Package plan defines the Plan aggregate.
package plan

import (
	"errors"
	"time"
)

var ErrEmptyName = errors.New("plan: name must not be empty")

// Plan is a reusable template for budget limits and model restrictions.
type Plan struct {
	ID               string
	TenantID         string
	Name             string
	Description      string
	LimitMicros      int64
	LimitTokens      int64
	RPMLimit         int64
	TPMLimit         int64
	ConcurrencyLimit int64
	Period           string
	AlertPct         int
	HardCutoff       bool
	AllowedModels    string // comma-separated patterns
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NewPlan constructs a Plan with required fields validated.
func NewPlan(id, tenantID, name string, now time.Time) (*Plan, error) {
	if name == "" {
		return nil, ErrEmptyName
	}
	return &Plan{
		ID:        id,
		TenantID:  tenantID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// UpdateLimits modifies the plan's budget limits.
func (p *Plan) UpdateLimits(tokenLimit, costLimit *int64, rateLimit *int64, now time.Time) {
	if tokenLimit != nil {
		p.LimitTokens = *tokenLimit
	}
	if costLimit != nil {
		p.LimitMicros = *costLimit
	}
	if rateLimit != nil {
		p.RPMLimit = *rateLimit
	}
	p.UpdatedAt = now
}
