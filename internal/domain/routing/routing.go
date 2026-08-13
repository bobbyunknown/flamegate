// Package routing defines routing domain types.
package routing

import "time"

// Target is a candidate provider/model for dispatch.
type Target struct {
	Provider  string
	Model     string
	AccountID string
	Priority  int
}

// Attempt records the outcome of one routing attempt.
type Attempt struct {
	Target    Target
	Success   bool
	Error     string
	LatencyMs int
}

// Chain is an ordered fallback definition.
type Chain struct {
	ID               string
	TenantID         string
	Name             string
	Strategy         string
	FallbackProvider string
	FallbackModel    string
	Targets          []Target
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
