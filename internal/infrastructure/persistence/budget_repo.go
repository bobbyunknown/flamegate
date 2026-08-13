package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type BudgetRepo struct{ db *gorm.DB }

func NewBudgetRepo(db *gorm.DB) *BudgetRepo { return &BudgetRepo{db: db} }

func (r *BudgetRepo) Create(ctx context.Context, b schema.Budget) error {
	return r.db.WithContext(ctx).Create(&b).Error
}
func (r *BudgetRepo) CreateOnTx(ctx context.Context, tx *gorm.DB, b schema.Budget) error {
	return tx.WithContext(ctx).Create(&b).Error
}
func (r *BudgetRepo) Get(ctx context.Context, id string) (schema.Budget, error) {
	var b schema.Budget
	err := r.db.WithContext(ctx).First(&b, "id = ?", id).Error
	return b, err
}
func (r *BudgetRepo) ListByScope(ctx context.Context, kind schema.BudgetScope, scopeID string) ([]schema.Budget, error) {
	var budgets []schema.Budget
	err := r.db.WithContext(ctx).Where("scope_kind = ? AND scope_id = ?", kind, scopeID).Find(&budgets).Error
	return budgets, err
}
func (r *BudgetRepo) ListByTenant(ctx context.Context, tenantID string) ([]schema.Budget, error) {
	var budgets []schema.Budget
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&budgets).Error
	return budgets, err
}
func (r *BudgetRepo) Update(ctx context.Context, b schema.Budget) error {
	return r.db.WithContext(ctx).Save(&b).Error
}
func (r *BudgetRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.Budget{}, "id = ?", id).Error
}
