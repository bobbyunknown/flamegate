package schema

// ---------------------------------------------------------------------------
// Value types and constants — migrated from adapters/store/models.go
// These are shared across the codebase and live here temporarily until
// domain/ sub-packages absorb them in refactor tahap 2.
// ---------------------------------------------------------------------------

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// DefaultTenantID is the implicit tenant used in local single-user mode.
const DefaultTenantID = "default"

// AuthKind classifies how an account authenticates upstream.
type AuthKind string

const (
	AuthAPIKey AuthKind = "api_key"
	AuthOAuth  AuthKind = "oauth"
	AuthNone   AuthKind = "none"
)

// BudgetScope identifies what a budget applies to.
type BudgetScope string

const (
	ScopeTenant  BudgetScope = "tenant"
	ScopeProject BudgetScope = "project"
	ScopeAPIKey  BudgetScope = "api_key"
)

// GuardrailScope identifies which dimension of a request a policy targets.
type GuardrailScope string

const (
	GuardrailScopeGlobal   GuardrailScope = "global"
	GuardrailScopeProvider GuardrailScope = "provider"
	GuardrailScopeModel    GuardrailScope = "model"
	GuardrailScopeChain    GuardrailScope = "chain"
	GuardrailScopeAPIKey   GuardrailScope = "apikey"
)

// Dialect identifies the active SQL engine.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

// ---------------------------------------------------------------------------
// Query aggregate types — used by usage/budget/query services
// ---------------------------------------------------------------------------

// SpendScope identifies a scope for batch spend queries.
type SpendScope struct {
	Kind    BudgetScope
	ScopeID string
	Since   time.Time
}

// SpendResult holds the result of a single scope's spend query.
type SpendResult struct {
	CostMicros int64
	Tokens     int64
}

type Summary struct {
	TotalRequests       int     `json:"total_requests"`
	TotalTokens         int64   `json:"total_tokens"`
	PromptTokens        int64   `json:"prompt_tokens"`
	CompletionTokens    int64   `json:"completion_tokens"`
	TotalCostMicros     int64   `json:"total_cost_micros"`
	CostMicros          int64   `json:"cost_micros"`
	CachedTokens        int64   `json:"cached_tokens"`
	CacheWriteTokens    int64   `json:"cache_write_tokens"`
	CacheHits           int64   `json:"cache_hits"`
	SuccessCount        int64   `json:"success_count"`
	SlimBytesSaved      int64   `json:"slim_bytes_saved"`
	SlimTokensSaved     int64   `json:"slim_tokens_saved"`
	HeadroomTokensSaved int64   `json:"headroom_tokens_saved"`
	HeadroomBytesSaved  int64   `json:"headroom_bytes_saved"`
	CavemanRequests     int64   `json:"caveman_requests"`
	TerseRequests       int64   `json:"terse_requests"`
	HeadroomRequests    int64   `json:"headroom_requests"`
	PonytailRequests    int64   `json:"ponytail_requests"`
	AvgLatencyMS        float64 `json:"avg_latency_ms"`
	AvgTTFTMS           float64 `json:"avg_ttft_ms"`
}

type ProviderUsage struct {
	Provider         string `json:"provider"`
	TotalRequests    int    `json:"total_requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	Tokens           int64  `json:"tokens"`
	CostMicros       int64  `json:"cost_micros"`
}

type RecentRecord struct {
	ID                  string    `json:"id"`
	TenantID            string    `json:"tenant_id"`
	APIKeyID            string    `json:"api_key_id"`
	Provider            string    `json:"provider"`
	Model               string    `json:"model"`
	PromptTokens        int       `json:"prompt_tokens"`
	CompletionTokens    int       `json:"completion_tokens"`
	CostMicros          int64     `json:"cost_micros"`
	CacheHit            bool      `json:"cache_hit"`
	LatencyMS           int       `json:"latency_ms"`
	TTFTMS              int       `json:"ttft_ms"`
	SlimBytesSaved      int       `json:"slim_bytes_saved"`
	SlimTokensSaved     int       `json:"slim_tokens_saved"`
	SlimRules           string    `json:"slim_rules"`
	CavemanActive       bool      `json:"caveman_active"`
	TerseActive         bool      `json:"terse_active"`
	HeadroomTokensSaved int       `json:"headroom_tokens_saved"`
	HeadroomBytesSaved  int       `json:"headroom_bytes_saved"`
	HeadroomActive      bool      `json:"headroom_active"`
	PonytailActive      bool      `json:"ponytail_active"`
	CreatedAt           time.Time `json:"created_at"`
}

// AccountUsage aggregates usage keyed by upstream account.
type AccountUsage struct {
	AccountID        string `json:"account_id"`
	TotalRequests    int    `json:"total_requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CachedTokens     int64  `json:"cached_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	Tokens           int64  `json:"tokens"`
	CostMicros       int64  `json:"cost_micros"`
}

// ModelUsage aggregates usage for a single provider+model pair.
type ModelUsage struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	TotalRequests    int    `json:"total_requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	CostMicros       int64  `json:"cost_micros"`
}

// TimeBucket represents an aggregated count of events for a specific time slice.
type TimeBucket struct {
	Bucket     int     `json:"bucket"`
	Count      int64   `json:"count"`
	Requests   int     `json:"requests"`
	CostMicros int64   `json:"cost_micros"`
	AvgLatency float64 `json:"avg_latency"`
}

// DailyPoint represents one day of usage for a specific API key.
type DailyPoint struct {
	Date             string `json:"date"`
	Requests         int    `json:"requests"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CostMicros       int64  `json:"cost_micros"`
}

// RuleSavings aggregates savings per RTK slimmer rule.
type RuleSavings struct {
	Rule        string `json:"rule"`
	Count       int    `json:"count"`
	TokensSaved int64  `json:"tokens_saved"`
	BytesSaved  int64  `json:"bytes_saved"`
}

// ClientSavings aggregates token-saving optimization results per calling client.
type ClientSavings struct {
	Client              string `json:"client"`
	Requests            int    `json:"requests"`
	SlimBytesSaved      int64  `json:"slim_bytes_saved"`
	SlimTokensSaved     int64  `json:"slim_tokens_saved"`
	HeadroomTokensSaved int64  `json:"headroom_tokens_saved"`
	HeadroomBytesSaved  int64  `json:"headroom_bytes_saved"`
	CavemanRequests     int64  `json:"caveman_requests"`
	TerseRequests       int64  `json:"terse_requests"`
	PonytailRequests    int64  `json:"ponytail_requests"`
}

// GuardrailLogFilter narrows audit log queries. Empty fields are ignored.
type GuardrailLogFilter struct {
	Detector string
	Provider string
	Model    string
	Action   string
	APIKeyID string
	Since    *time.Time
	Limit    int
}

// GetPlanAllowedModels returns the allowed models for a plan as a slice.
func GetPlanAllowedModels(p Plan) []string {
	if p.AllowedModels == "" {
		return nil
	}
	var out []string
	start := 0
	for i := range len(p.AllowedModels) {
		if p.AllowedModels[i] == ',' {
			out = append(out, p.AllowedModels[start:i])
			start = i + 1
		}
	}
	out = append(out, p.AllowedModels[start:])
	return out
}

// SetPlanAllowedModels converts a slice of model names into the
// csv string stored in AllowedModels.
func SetPlanAllowedModels(models []string) string {
	if len(models) == 0 {
		return ""
	}
	out := models[0]
	for _, m := range models[1:] {
		out += "," + m
	}
	return out
}
