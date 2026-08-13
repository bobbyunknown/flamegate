package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func accountHealthJSON(h schema.AccountHealth) map[string]any {
	row := map[string]any{
		"id":                    h.ID,
		"tenant_id":             h.TenantID,
		"account_id":            h.AccountID,
		"provider":              h.Provider,
		"model":                 h.Model,
		"status":                h.Status,
		"latency_ms":            h.LatencyMS,
		"consecutive_failures":  h.ConsecutiveFailures,
		"consecutive_successes": h.ConsecutiveSuccesses,
		"last_checked_at":       h.LastCheckedAt,
		"last_error":            h.LastError,
		"updated_at":            h.UpdatedAt,
	}
	if h.LastOKAt != nil {
		row["last_ok_at"] = *h.LastOKAt
	}
	return row
}

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = 30 * time.Second
	}
	return context.WithTimeout(r.Context(), d)
}
