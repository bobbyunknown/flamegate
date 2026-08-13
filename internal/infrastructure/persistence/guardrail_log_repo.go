package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type GuardrailLogRepo struct{ db *gorm.DB }

func NewGuardrailLogRepo(db *gorm.DB) *GuardrailLogRepo { return &GuardrailLogRepo{db: db} }

func (r *GuardrailLogRepo) Insert(ctx context.Context, e schema.GuardrailLog) error {
	return r.db.WithContext(ctx).Create(&e).Error
}
func (r *GuardrailLogRepo) BatchInsert(ctx context.Context, entries []schema.GuardrailLog) error {
	return r.db.WithContext(ctx).CreateInBatches(entries, 100).Error
}
func (r *GuardrailLogRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&schema.GuardrailLog{})
	return result.RowsAffected, result.Error
}
func (r *GuardrailLogRepo) List(ctx context.Context, tenantID string, f schema.GuardrailLogFilter) ([]schema.GuardrailLog, error) {
	var logs []schema.GuardrailLog
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at DESC")
	if f.Detector != "" {
		q = q.Where("detector = ?", f.Detector)
	}
	if f.Action != "" {
		q = q.Where("action = ?", f.Action)
	}
	if f.APIKeyID != "" {
		q = q.Where("api_key_id = ?", f.APIKeyID)
	}
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	err := q.Find(&logs).Error
	return logs, err
}
