package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type AccountRepo struct{ db *gorm.DB }

func NewAccountRepo(db *gorm.DB) *AccountRepo { return &AccountRepo{db: db} }

func (r *AccountRepo) Create(ctx context.Context, a schema.Account) error {
	return r.db.WithContext(ctx).Create(&a).Error
}
func (r *AccountRepo) Get(ctx context.Context, id string) (schema.Account, error) {
	var a schema.Account
	err := r.db.WithContext(ctx).First(&a, "id = ?", id).Error
	return a, err
}
func (r *AccountRepo) ListByProvider(ctx context.Context, tenantID, provider string) ([]schema.Account, error) {
	var accounts []schema.Account
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND provider = ?", tenantID, provider).Order("priority ASC").Find(&accounts).Error
	return accounts, err
}
func (r *AccountRepo) ListByTenant(ctx context.Context, tenantID string) ([]schema.Account, error) {
	var accounts []schema.Account
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&accounts).Error
	return accounts, err
}
func (r *AccountRepo) Update(ctx context.Context, a schema.Account) error {
	return r.db.WithContext(ctx).Save(&a).Error
}
func (r *AccountRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.Account{}, "id = ?", id).Error
}
func (r *AccountRepo) SetCooldown(ctx context.Context, id string, until time.Time) error {
	return r.db.WithContext(ctx).Model(&schema.Account{}).Where("id = ?", id).Updates(map[string]any{"cooldown_until": until, "status": "cooldown"}).Error
}
func (r *AccountRepo) SetBackoffLevel(ctx context.Context, id string, level int) error {
	return r.db.WithContext(ctx).Model(&schema.Account{}).Where("id = ?", id).Update("backoff_level", level).Error
}
func (r *AccountRepo) ResetBackoffLevel(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&schema.Account{}).Where("id = ?", id).Updates(map[string]any{"backoff_level": 0, "cooldown_until": nil}).Error
}
func (r *AccountRepo) ClearExpiredCooldowns(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Model(&schema.Account{}).Where("cooldown_until IS NOT NULL AND cooldown_until < ?", time.Now()).Updates(map[string]any{"cooldown_until": nil, "backoff_level": 0})
	return result.RowsAffected, result.Error
}
func (r *AccountRepo) ClearProviderCooldowns(ctx context.Context, tenantID, provider string) error {
	return r.db.WithContext(ctx).Model(&schema.Account{}).Where("tenant_id = ? AND provider = ?", tenantID, provider).Updates(map[string]any{"cooldown_until": nil, "backoff_level": 0, "needs_reconnect": false}).Error
}
func (r *AccountRepo) UpdateTokens(ctx context.Context, a schema.Account) error {
	return r.db.WithContext(ctx).Model(&schema.Account{}).Where("id = ?", a.ID).Updates(map[string]any{"token_wrapped_dek": a.TokenWrappedDEK, "token_ciphertext": a.TokenCiphertext, "token_expires_at": a.TokenExpiresAt, "needs_reconnect": false}).Error
}
func (r *AccountRepo) SetNeedsReconnect(ctx context.Context, id string, flag bool) error {
	return r.db.WithContext(ctx).Model(&schema.Account{}).Where("id = ?", id).Update("needs_reconnect", flag).Error
}
