package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type CustomProviderRepo struct{ db *gorm.DB }

func NewCustomProviderRepo(db *gorm.DB) *CustomProviderRepo { return &CustomProviderRepo{db: db} }

func (r *CustomProviderRepo) ListProviders(ctx context.Context, tenantID string) ([]schema.CustomProvider, error) {
	var providers []schema.CustomProvider
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&providers).Error
	return providers, err
}
func (r *CustomProviderRepo) GetProvider(ctx context.Context, id string) (schema.CustomProvider, error) {
	var p schema.CustomProvider
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	return p, err
}
func (r *CustomProviderRepo) CreateProvider(ctx context.Context, p schema.CustomProvider) error {
	return r.db.WithContext(ctx).Create(&p).Error
}
func (r *CustomProviderRepo) UpdateProvider(ctx context.Context, p schema.CustomProvider) error {
	return r.db.WithContext(ctx).Save(&p).Error
}
func (r *CustomProviderRepo) DeleteProvider(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_id = ?", id).Delete(&schema.CustomModel{}).Error; err != nil {
			return err
		}
		return tx.Delete(&schema.CustomProvider{}, "id = ?", id).Error
	})
}
func (r *CustomProviderRepo) ListModels(ctx context.Context, tenantID string) ([]schema.CustomModel, error) {
	var models []schema.CustomModel
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&models).Error
	return models, err
}
func (r *CustomProviderRepo) ListModelsByProvider(ctx context.Context, providerID string) ([]schema.CustomModel, error) {
	var models []schema.CustomModel
	err := r.db.WithContext(ctx).Where("provider_id = ?", providerID).Find(&models).Error
	return models, err
}
func (r *CustomProviderRepo) CreateModel(ctx context.Context, m schema.CustomModel) error {
	return r.db.WithContext(ctx).Create(&m).Error
}
func (r *CustomProviderRepo) UpdateModel(ctx context.Context, m schema.CustomModel) error {
	return r.db.WithContext(ctx).Save(&m).Error
}
func (r *CustomProviderRepo) GetModel(ctx context.Context, id string) (schema.CustomModel, error) {
	var m schema.CustomModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	return m, err
}
func (r *CustomProviderRepo) DeleteModel(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.CustomModel{}, "id = ?", id).Error
}
