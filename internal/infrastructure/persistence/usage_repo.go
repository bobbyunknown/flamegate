package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type UsageRepo struct{ db *gorm.DB }

func NewUsageRepo(db *gorm.DB) *UsageRepo { return &UsageRepo{db: db} }

func (r *UsageRepo) Record(ctx context.Context, u schema.UsageRecord) error {
	return r.db.WithContext(ctx).Create(&u).Error
}
func (r *UsageRepo) RecordBatch(ctx context.Context, records []schema.UsageRecord) error {
	return r.db.WithContext(ctx).CreateInBatches(records, 100).Error
}
func (r *UsageRepo) SpendSince(ctx context.Context, scope schema.BudgetScope, scopeID string, since time.Time) (int64, error) {
	var total int64
	col := scopeColumn(scope)
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where(col+" = ? AND created_at >= ?", scopeID, since).Select("COALESCE(SUM(cost_micros), 0)").Scan(&total).Error
	return total, err
}
func (r *UsageRepo) SpendAndTokens(ctx context.Context, scope schema.BudgetScope, scopeID string, since time.Time) (int64, int64, error) {
	var result struct {
		CostMicros int64
		Tokens     int64
	}
	col := scopeColumn(scope)
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where(col+" = ? AND created_at >= ?", scopeID, since).Select("COALESCE(SUM(cost_micros), 0) as cost_micros, COALESCE(SUM(prompt_tokens + completion_tokens), 0) as tokens").Scan(&result).Error
	return result.CostMicros, result.Tokens, err
}
func (r *UsageRepo) SpendAndTokensBatch(ctx context.Context, scopes []schema.SpendScope) ([]schema.SpendResult, error) {
	results := make([]schema.SpendResult, 0, len(scopes))
	for _, s := range scopes {
		cost, tokens, err := r.SpendAndTokens(ctx, s.Kind, s.ScopeID, s.Since)
		if err != nil {
			return nil, err
		}
		results = append(results, schema.SpendResult{CostMicros: cost, Tokens: tokens})
	}
	return results, nil
}
func (r *UsageRepo) Summarize(ctx context.Context, tenantID string, since time.Time) (schema.Summary, error) {
	var s schema.Summary
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("tenant_id = ? AND created_at >= ?", tenantID, since).Select("COUNT(*) as total_requests, COALESCE(SUM(prompt_tokens + completion_tokens), 0) as total_tokens, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(cost_micros), 0) as total_cost_micros, COALESCE(SUM(cost_micros), 0) as cost_micros, COALESCE(SUM(cached_tokens), 0) as cached_tokens, COALESCE(SUM(cache_write_tokens), 0) as cache_write_tokens, COALESCE(SUM(CASE WHEN cache_hit THEN 1 ELSE 0 END), 0) as cache_hits, COALESCE(SUM(slim_bytes_saved), 0) as slim_bytes_saved, COALESCE(SUM(slim_tokens_saved), 0) as slim_tokens_saved, COALESCE(SUM(headroom_tokens_saved), 0) as headroom_tokens_saved, COALESCE(SUM(headroom_bytes_saved), 0) as headroom_bytes_saved, COALESCE(SUM(CASE WHEN cache_hit OR latency_ms > 0 THEN 1 ELSE 0 END), 0) as success_count, COALESCE(AVG(latency_ms), 0) as avg_latency_ms, COALESCE(AVG(ttft_ms), 0) as avg_ttft_ms").Scan(&s).Error
	return s, err
}
func (r *UsageRepo) SummarizeByKey(ctx context.Context, keyID string, since time.Time) (schema.Summary, error) {
	var s schema.Summary
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("api_key_id = ? AND created_at >= ?", keyID, since).Select("COUNT(*) as total_requests, COALESCE(SUM(prompt_tokens + completion_tokens), 0) as total_tokens, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(cost_micros), 0) as total_cost_micros, COALESCE(SUM(cost_micros), 0) as cost_micros, COALESCE(SUM(cached_tokens), 0) as cached_tokens, COALESCE(SUM(cache_write_tokens), 0) as cache_write_tokens, COALESCE(SUM(CASE WHEN cache_hit THEN 1 ELSE 0 END), 0) as cache_hits, COALESCE(SUM(slim_bytes_saved), 0) as slim_bytes_saved, COALESCE(SUM(slim_tokens_saved), 0) as slim_tokens_saved, COALESCE(SUM(headroom_tokens_saved), 0) as headroom_tokens_saved, COALESCE(SUM(headroom_bytes_saved), 0) as headroom_bytes_saved, COALESCE(SUM(CASE WHEN cache_hit OR latency_ms > 0 THEN 1 ELSE 0 END), 0) as success_count, COALESCE(AVG(latency_ms), 0) as avg_latency_ms, COALESCE(AVG(ttft_ms), 0) as avg_ttft_ms").Scan(&s).Error
	return s, err
}
func (r *UsageRepo) Breakdown(ctx context.Context, tenantID string, since time.Time) ([]schema.ProviderUsage, error) {
	var results []schema.ProviderUsage
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("tenant_id = ? AND created_at >= ?", tenantID, since).Select("provider, COUNT(*) as total_requests, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(prompt_tokens + completion_tokens), 0) as tokens, COALESCE(SUM(cost_micros), 0) as cost_micros").Group("provider").Order("total_requests DESC").Scan(&results).Error
	return results, err
}
func (r *UsageRepo) Recent(ctx context.Context, tenantID string, limit int) ([]schema.RecentRecord, error) {
	var records []schema.RecentRecord
	q := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("tenant_id = ?", tenantID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Select("id, tenant_id, api_key_id, provider, model, prompt_tokens, completion_tokens, cost_micros, cache_hit, latency_ms, ttft_ms, slim_bytes_saved, slim_tokens_saved, slim_rules, caveman_active, terse_active, headroom_tokens_saved, headroom_bytes_saved, headroom_active, ponytail_active, created_at").Find(&records).Error
	return records, err
}
func (r *UsageRepo) ByAccount(ctx context.Context, tenantID string, since time.Time) ([]schema.AccountUsage, error) {
	var results []schema.AccountUsage
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("tenant_id = ? AND created_at >= ?", tenantID, since).Select("account_id, COUNT(*) as requests, COALESCE(SUM(prompt_tokens + completion_tokens), 0) as tokens, COALESCE(SUM(cost_micros), 0) as cost_micros").Group("account_id").Scan(&results).Error
	return results, err
}
func (r *UsageRepo) ByModel(ctx context.Context, tenantID string, since time.Time) ([]schema.ModelUsage, error) {
	var results []schema.ModelUsage
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("tenant_id = ? AND created_at >= ?", tenantID, since).Select("provider, model, COUNT(*) as total_requests, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(cost_micros), 0) as cost_micros").Group("provider, model").Order("total_requests DESC").Scan(&results).Error
	return results, err
}
func (r *UsageRepo) ByModelByKey(ctx context.Context, keyID string, since time.Time) ([]schema.ModelUsage, error) {
	var results []schema.ModelUsage
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("api_key_id = ? AND created_at >= ?", keyID, since).Select("provider, model, COUNT(*) as total_requests, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(cost_micros), 0) as cost_micros").Group("provider, model").Order("total_requests DESC").Scan(&results).Error
	return results, err
}
func (r *UsageRepo) Timeline(ctx context.Context, tenantID string, since, to time.Time, buckets int) ([]schema.TimeBucket, error) {
	var results []schema.TimeBucket
	// Simple implementation: query all records in range and bucket in Go
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("tenant_id = ? AND created_at BETWEEN ? AND ?", tenantID, since, to).Select("created_at, cost_micros").Order("created_at ASC").Scan(&results).Error
	return results, err
}
func (r *UsageRepo) DailyByKey(ctx context.Context, keyID string, since time.Time) ([]schema.DailyPoint, error) {
	var results []schema.DailyPoint
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("api_key_id = ? AND created_at >= ?", keyID, since).Select("DATE(created_at) as date, COUNT(*) as requests, COALESCE(SUM(prompt_tokens), 0) as prompt_tokens, COALESCE(SUM(completion_tokens), 0) as completion_tokens, COALESCE(SUM(cost_micros), 0) as cost_micros").Group("DATE(created_at)").Order("date ASC").Scan(&results).Error
	return results, err
}
func (r *UsageRepo) SavingsByRule(ctx context.Context, tenantID string, since time.Time) ([]schema.RuleSavings, error) {
	var results []schema.RuleSavings
	// Unnest the slim_rules CSV field and aggregate per rule
	err := r.db.WithContext(ctx).Raw(`
		SELECT TRIM(value) AS rule,
		       COUNT(*) AS count,
		       COALESCE(SUM(slim_tokens_saved), 0) AS tokens_saved,
		       COALESCE(SUM(slim_bytes_saved), 0) AS bytes_saved
		FROM usage_records, json_each('["' || REPLACE(slim_rules, ',', '","') || '"]')
		WHERE tenant_id = ? AND created_at >= ? AND slim_rules != ''
		GROUP BY TRIM(value)
		ORDER BY bytes_saved DESC
	`, tenantID, since).Scan(&results).Error
	if err != nil {
		return nil, nil // return empty on unnest failure; caller treats nil as no rows
	}
	return results, nil
}
func (r *UsageRepo) SavingsByClient(ctx context.Context, tenantID string, since time.Time) ([]schema.ClientSavings, error) {
	var results []schema.ClientSavings
	err := r.db.WithContext(ctx).Model(&schema.UsageRecord{}).Where("tenant_id = ? AND created_at >= ?", tenantID, since).Select(
		"client, COUNT(*) as requests, COALESCE(SUM(slim_bytes_saved), 0) as slim_bytes_saved, COALESCE(SUM(slim_tokens_saved), 0) as slim_tokens_saved, COALESCE(SUM(headroom_tokens_saved), 0) as headroom_tokens_saved, COALESCE(SUM(headroom_bytes_saved), 0) as headroom_bytes_saved, COALESCE(SUM(CASE WHEN caveman_active THEN 1 ELSE 0 END), 0) as caveman_requests, COALESCE(SUM(CASE WHEN terse_active THEN 1 ELSE 0 END), 0) as terse_requests, COALESCE(SUM(CASE WHEN ponytail_active THEN 1 ELSE 0 END), 0) as ponytail_requests",
	).Group("client").Order("requests DESC").Scan(&results).Error
	return results, err
}

func scopeColumn(scope schema.BudgetScope) string {
	switch scope {
	case schema.ScopeTenant:
		return "tenant_id"
	case schema.ScopeProject:
		return "project_id"
	case schema.ScopeAPIKey:
		return "api_key_id"
	default:
		return "tenant_id"
	}
}
