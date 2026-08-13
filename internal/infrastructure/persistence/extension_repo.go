package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// ExtensionRepo implements ports.ExtensionRepository.
type ExtensionRepo struct{ db *gorm.DB }

// NewExtensionRepo creates a new ExtensionRepo.
func NewExtensionRepo(db *gorm.DB) *ExtensionRepo { return &ExtensionRepo{db: db} }

func (r *ExtensionRepo) Get(ctx context.Context, id string) (schema.Extension, error) {
	var e schema.Extension
	err := r.db.WithContext(ctx).First(&e, "id = ?", id).Error
	return e, err
}

func (r *ExtensionRepo) FindBySlug(ctx context.Context, slug string) (schema.Extension, error) {
	var e schema.Extension
	err := r.db.WithContext(ctx).First(&e, "slug = ?", slug).Error
	return e, err
}

func (r *ExtensionRepo) List(ctx context.Context, tenantID string) ([]schema.Extension, error) {
	var exts []schema.Extension
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&exts).Error
	return exts, err
}

func (r *ExtensionRepo) ListByState(ctx context.Context, state string) ([]schema.Extension, error) {
	var exts []schema.Extension
	err := r.db.WithContext(ctx).Where("state = ?", state).Find(&exts).Error
	return exts, err
}

func (r *ExtensionRepo) Create(ctx context.Context, e schema.Extension) error {
	return r.db.WithContext(ctx).Create(&e).Error
}

func (r *ExtensionRepo) Update(ctx context.Context, e schema.Extension) error {
	return r.db.WithContext(ctx).Save(&e).Error
}

func (r *ExtensionRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.Extension{}, "id = ?", id).Error
}
