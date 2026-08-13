package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type HealthRepo struct{ db *gorm.DB }

func NewHealthRepo(db *gorm.DB) *HealthRepo { return &HealthRepo{db: db} }

func (r *HealthRepo) Upsert(ctx context.Context, h schema.AccountHealth) error {
	return r.db.WithContext(ctx).Save(&h).Error
}
func (r *HealthRepo) Get(ctx context.Context, accountID, model string) (schema.AccountHealth, error) {
	var h schema.AccountHealth
	err := r.db.WithContext(ctx).Where("account_id = ? AND model = ?", accountID, model).First(&h).Error
	return h, err
}
func (r *HealthRepo) IsUnhealthy(ctx context.Context, accountID, model string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&schema.AccountHealth{}).Where("account_id = ? AND (model = ? OR model = '__all__') AND status = 'unhealthy'", accountID, model).Count(&count).Error
	return count > 0, err
}
func (r *HealthRepo) MarkHealthy(ctx context.Context, accountID, model string) error {
	return r.db.WithContext(ctx).Model(&schema.AccountHealth{}).Where("account_id = ? AND model = ?", accountID, model).Update("status", "healthy").Error
}
func (r *HealthRepo) List(ctx context.Context, tenantID string) ([]schema.AccountHealth, error) {
	var rows []schema.AccountHealth
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&rows).Error
	return rows, err
}
func (r *HealthRepo) RecentAccountModels(ctx context.Context, tenantID string, since time.Time, limit int) ([]schema.AccountHealth, error) {
	var rows []schema.AccountHealth
	q := r.db.WithContext(ctx).Where("tenant_id = ? AND last_checked_at >= ?", tenantID, since)
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rows).Error
	return rows, err
}
