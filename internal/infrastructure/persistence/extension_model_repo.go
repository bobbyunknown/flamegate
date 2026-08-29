package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// ExtensionModelRepo implements ports.ExtensionModelRepository.
type ExtensionModelRepo struct{ db *gorm.DB }

// NewExtensionModelRepo creates a new ExtensionModelRepo.
func NewExtensionModelRepo(db *gorm.DB) *ExtensionModelRepo {
	return &ExtensionModelRepo{db: db}
}

func (r *ExtensionModelRepo) Get(ctx context.Context, id string) (schema.ExtensionModel, error) {
	var m schema.ExtensionModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	return m, err
}

func (r *ExtensionModelRepo) ListByExtension(ctx context.Context, extensionID string) ([]schema.ExtensionModel, error) {
	var models []schema.ExtensionModel
	err := r.db.WithContext(ctx).Where("extension_id = ?", extensionID).Find(&models).Error
	return models, err
}

func (r *ExtensionModelRepo) ListBySource(ctx context.Context, extensionID, source string) ([]schema.ExtensionModel, error) {
	var models []schema.ExtensionModel
	err := r.db.WithContext(ctx).Where("extension_id = ? AND source = ?", extensionID, source).Find(&models).Error
	return models, err
}

func (r *ExtensionModelRepo) ListByTenant(ctx context.Context, tenantID string) ([]schema.ExtensionModel, error) {
	var models []schema.ExtensionModel
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&models).Error
	return models, err
}

func (r *ExtensionModelRepo) Create(ctx context.Context, m schema.ExtensionModel) error {
	return r.db.WithContext(ctx).Create(&m).Error
}

func (r *ExtensionModelRepo) Update(ctx context.Context, m schema.ExtensionModel) error {
	return r.db.WithContext(ctx).Save(&m).Error
}

func (r *ExtensionModelRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.ExtensionModel{}, "id = ?", id).Error
}

func (r *ExtensionModelRepo) DeleteBySlug(ctx context.Context, extensionID, slug string) error {
	return r.db.WithContext(ctx).Where("extension_id = ? AND (slug = ? OR id = ?)", extensionID, slug, extensionID+"/"+slug).
		Delete(&schema.ExtensionModel{}).Error
}

func (r *ExtensionModelRepo) DeleteBySource(ctx context.Context, extensionID, source string) error {
	return r.db.WithContext(ctx).Where("extension_id = ? AND source = ?", extensionID, source).
		Delete(&schema.ExtensionModel{}).Error
}

func (r *ExtensionModelRepo) DeleteByExtension(ctx context.Context, extensionID string) error {
	return r.db.WithContext(ctx).Where("extension_id = ?", extensionID).
		Delete(&schema.ExtensionModel{}).Error
}
