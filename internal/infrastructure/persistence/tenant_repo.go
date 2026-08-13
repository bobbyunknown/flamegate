package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type TenantRepo struct{ db *gorm.DB }

func NewTenantRepo(db *gorm.DB) *TenantRepo { return &TenantRepo{db: db} }

func (r *TenantRepo) EnsureDefault(ctx context.Context) error {
	return r.db.WithContext(ctx).FirstOrCreate(&schema.Tenant{ID: "default", Name: "Default"}).Error
}
func (r *TenantRepo) Upsert(ctx context.Context, t schema.Tenant) error {
	return r.db.WithContext(ctx).Save(&t).Error
}
func (r *TenantRepo) Get(ctx context.Context, id string) (schema.Tenant, error) {
	var t schema.Tenant
	err := r.db.WithContext(ctx).First(&t, "id = ?", id).Error
	return t, err
}
