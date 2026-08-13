package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type GuardrailRepo struct{ db *gorm.DB }

func NewGuardrailRepo(db *gorm.DB) *GuardrailRepo { return &GuardrailRepo{db: db} }

func (r *GuardrailRepo) Upsert(ctx context.Context, p schema.GuardrailPolicy) error {
	return r.db.WithContext(ctx).Save(&p).Error
}
func (r *GuardrailRepo) Get(ctx context.Context, id string) (schema.GuardrailPolicy, error) {
	var p schema.GuardrailPolicy
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	return p, err
}
func (r *GuardrailRepo) GetByScope(ctx context.Context, tenantID string, scope schema.GuardrailScope, scopeID string) (schema.GuardrailPolicy, error) {
	var p schema.GuardrailPolicy
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND scope = ? AND scope_id = ?", tenantID, string(scope), scopeID).First(&p).Error
	return p, err
}
func (r *GuardrailRepo) List(ctx context.Context, tenantID, scope string) ([]schema.GuardrailPolicy, error) {
	var policies []schema.GuardrailPolicy
	q := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if scope != "" {
		q = q.Where("scope = ?", scope)
	}
	err := q.Order("created_at DESC").Find(&policies).Error
	return policies, err
}
func (r *GuardrailRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.GuardrailPolicy{}, "id = ?", id).Error
}
