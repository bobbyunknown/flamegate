// Package usage defines usage metering domain types.
package usage

import "time"

// Entry is one metered request.
type Entry struct {
	ID           string
	TenantID     string
	KeyID        string
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
	CostMicros   int64
	CacheHit     bool
	LatencyMs    int
	CreatedAt    time.Time
}

// Snapshot aggregates usage for a period.
type Snapshot struct {
	TotalTokens int64
	TotalCost   int64
	Period      string
}
