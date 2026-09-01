package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// APIKeyRepo implements ports.APIKeyRepository using GORM.
type APIKeyRepo struct {
	db *gorm.DB
}

// NewAPIKeyRepo creates a new GORM-backed API key repository.
func NewAPIKeyRepo(db *gorm.DB) *APIKeyRepo {
	return &APIKeyRepo{db: db}
}

// Create inserts a new API key.
func (r *APIKeyRepo) Create(ctx context.Context, k schema.APIKey) error {
	return r.db.WithContext(ctx).Create(&k).Error
}

// CreateOnTx inserts a key within a transaction.
func (r *APIKeyRepo) CreateOnTx(ctx context.Context, tx *gorm.DB, k schema.APIKey) error {
	return tx.WithContext(ctx).Create(&k).Error
}

// Get returns a single API key by ID.
func (r *APIKeyRepo) Get(ctx context.Context, id string) (schema.APIKey, error) {
	var k schema.APIKey
	err := r.db.WithContext(ctx).First(&k, "id = ?", id).Error
	return k, err
}

// FindByLookup returns the enabled key matching the lookup hash.
func (r *APIKeyRepo) FindByLookup(ctx context.Context, lookup string) (schema.APIKey, error) {
	var k schema.APIKey
	err := r.db.WithContext(ctx).Where("lookup_hash = ?", lookup).First(&k).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return k, schema.ErrNotFound
	}
	return k, err
}

// List returns all keys for a tenant, newest first.
func (r *APIKeyRepo) List(ctx context.Context, tenantID string) ([]schema.APIKey, error) {
	var keys []schema.APIKey
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&keys).Error
	return keys, err
}

// TouchLastUsed records that a key authenticated a request.
func (r *APIKeyRepo) TouchLastUsed(ctx context.Context, id string, at time.Time) error {
	return r.db.WithContext(ctx).Model(&schema.APIKey{}).Where("id = ?", id).Update("last_used_at", at).Error
}

// SetDisabled enables or disables a key.
func (r *APIKeyRepo) SetDisabled(ctx context.Context, id string, disabled bool) error {
	return r.db.WithContext(ctx).Model(&schema.APIKey{}).Where("id = ?", id).Update("disabled", disabled).Error
}

// Delete removes a key.
func (r *APIKeyRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.APIKey{}, "id = ?", id).Error
}

// SetPlanID updates the plan assignment for a key.
func (r *APIKeyRepo) SetPlanID(ctx context.Context, id string, planID string) error {
	return r.db.WithContext(ctx).Model(&schema.APIKey{}).Where("id = ?", id).Update("plan_id", planID).Error
}

// SetAllowedModels replaces the allowed-models list for a key.
func (r *APIKeyRepo) SetAllowedModels(ctx context.Context, keyID string, models []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("api_key_id = ?", keyID).Delete(&schema.APIKeyModelAccess{}).Error; err != nil {
			return err
		}
		for _, m := range models {
			access := schema.APIKeyModelAccess{APIKeyID: keyID, Model: m, CreatedAt: time.Now()}
			if err := tx.Create(&access).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// GetAllowedModels returns the models allowed for a key.
func (r *APIKeyRepo) GetAllowedModels(ctx context.Context, keyID string) ([]string, error) {
	var accesses []schema.APIKeyModelAccess
	if err := r.db.WithContext(ctx).Where("api_key_id = ?", keyID).Find(&accesses).Error; err != nil {
		return nil, err
	}
	models := make([]string, 0, len(accesses))
	for _, a := range accesses {
		models = append(models, a.Model)
	}
	return models, nil
}

// IsModelAllowed reports whether a model is permitted for the given key.
func (r *APIKeyRepo) IsModelAllowed(ctx context.Context, keyID string, model string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&schema.APIKeyModelAccess{}).
		Where("api_key_id = ? AND (model = ? OR model = ?)", keyID, model, model+"%").
		Count(&count).Error
	return count > 0, err
}

// SetAllowedModelsOnTx replaces allowed models within an existing transaction.
func (r *APIKeyRepo) SetAllowedModelsOnTx(ctx context.Context, tx *gorm.DB, keyID string, models []string) error {
	if err := tx.WithContext(ctx).Where("api_key_id = ?", keyID).Delete(&schema.APIKeyModelAccess{}).Error; err != nil {
		return err
	}
	for _, m := range models {
		access := schema.APIKeyModelAccess{APIKeyID: keyID, Model: m, CreatedAt: time.Now()}
		if err := tx.WithContext(ctx).Create(&access).Error; err != nil {
			return err
		}
	}
	return nil
}

// RotateSecret updates the key hash, lookup hash, and display for an existing API key.
func (r *APIKeyRepo) RotateSecret(ctx context.Context, id, keyHash, lookupHash, display string) error {
	return r.db.WithContext(ctx).Model(&schema.APIKey{}).Where("id = ?", id).Updates(map[string]any{
		"key_hash":    keyHash,
		"lookup_hash": lookupHash,
		"display":     display,
	}).Error
}
