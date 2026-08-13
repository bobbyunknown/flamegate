package query

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// UsageSummary represents aggregated usage data.
type UsageSummary struct {
	TotalRequests int   `json:"total_requests"`
	TotalTokens   int64 `json:"total_tokens"`
	TotalCost     int64 `json:"total_cost_micros"`
}

// UsageQuery provides read-only usage queries.
type UsageQuery struct {
	usageRepo ports.UsageRepository
}

// NewUsageQuery creates a new UsageQuery.
func NewUsageQuery(repo ports.UsageRepository) *UsageQuery {
	return &UsageQuery{usageRepo: repo}
}

// Summary returns aggregate usage for a tenant.
func (q *UsageQuery) Summary(ctx context.Context, tenantID, period string) (*UsageSummary, error) {
	return nil, nil
}
