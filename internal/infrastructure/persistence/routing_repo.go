package persistence

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type RoutingRepo struct{ db *gorm.DB }

func NewRoutingRepo(db *gorm.DB) *RoutingRepo { return &RoutingRepo{db: db} }

func (r *RoutingRepo) SetModelCooldown(ctx context.Context, accountID, model string, until time.Time) error {
	return r.db.WithContext(ctx).Save(&schema.ModelCooldown{
		ID: accountID + ":" + model, AccountID: accountID, Model: model, CooldownUntil: until,
	}).Error
}
func (r *RoutingRepo) ClearModelCooldown(ctx context.Context, accountID, model string) error {
	return r.db.WithContext(ctx).Where("account_id = ? AND model = ?", accountID, model).Delete(&schema.ModelCooldown{}).Error
}
func (r *RoutingRepo) ClearAccountModelCooldowns(ctx context.Context, accountID string) error {
	return r.db.WithContext(ctx).Where("account_id = ?", accountID).Delete(&schema.ModelCooldown{}).Error
}
func (r *RoutingRepo) IsModelCooldownActive(ctx context.Context, accountID, model string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&schema.ModelCooldown{}).Where("account_id = ? AND model = ? AND cooldown_until > ?", accountID, model, time.Now()).Count(&count).Error
	return count > 0, err
}
func (r *RoutingRepo) ExpireModelCooldowns(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Where("cooldown_until <= ?", time.Now()).Delete(&schema.ModelCooldown{})
	return result.RowsAffected, result.Error
}
func (r *RoutingRepo) GetChainRotation(ctx context.Context, chainID string) (int, error) {
	var s schema.ChainRotation
	err := r.db.WithContext(ctx).First(&s, "chain_id = ?", chainID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, schema.ErrNotFound
		}
		return 0, nil
	}
	return s.LastIndex, nil
}
func (r *RoutingRepo) SetChainRotation(ctx context.Context, chainID string, index int) error {
	return r.db.WithContext(ctx).Save(&schema.ChainRotation{ChainID: chainID, LastIndex: index, UpdatedAt: time.Now()}).Error
}
func (r *RoutingRepo) GetAccountAffinity(ctx context.Context, scopeKey string) (schema.AccountAffinity, error) {
	var a schema.AccountAffinity
	err := r.db.WithContext(ctx).First(&a, "scope_key = ?", scopeKey).Error
	return a, err
}
func (r *RoutingRepo) SetAccountAffinity(ctx context.Context, a schema.AccountAffinity) error {
	return r.db.WithContext(ctx).Save(&a).Error
}
func (r *RoutingRepo) ExpireAccountAffinities(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Where("expires_at <= ?", time.Now()).Delete(&schema.AccountAffinity{})
	return result.RowsAffected, result.Error
}

// GetChainRotationState returns the full rotation record for a chain.
func (r *RoutingRepo) GetChainRotationState(ctx context.Context, chainID string) (schema.ChainRotation, error) {
	var s schema.ChainRotation
	err := r.db.WithContext(ctx).First(&s, "chain_id = ?", chainID).Error
	return s, err
}

// SetChainRotationState upserts a chain rotation record.
func (r *RoutingRepo) SetChainRotationState(ctx context.Context, state schema.ChainRotation) error {
	return r.db.WithContext(ctx).Save(&state).Error
}

// GetTargetRotationState returns the rotation record for a provider/model target.
func (r *RoutingRepo) GetTargetRotationState(ctx context.Context, scopeKey string) (schema.TargetRotation, error) {
	var s schema.TargetRotation
	err := r.db.WithContext(ctx).First(&s, "scope_key = ?", scopeKey).Error
	return s, err
}

// SetTargetRotationState upserts a target rotation record.
func (r *RoutingRepo) SetTargetRotationState(ctx context.Context, state schema.TargetRotation) error {
	return r.db.WithContext(ctx).Save(&state).Error
}
