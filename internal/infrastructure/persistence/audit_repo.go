package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type AuditRepo struct{ db *gorm.DB }

func NewAuditRepo(db *gorm.DB) *AuditRepo { return &AuditRepo{db: db} }

func (r *AuditRepo) Append(ctx context.Context, e schema.AuditEntry) error {
	return r.db.WithContext(ctx).Create(&e).Error
}
func (r *AuditRepo) List(ctx context.Context, tenantID string, limit int) ([]schema.AuditEntry, error) {
	var entries []schema.AuditEntry
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&entries).Error
	return entries, err
}
