package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type PlanRepo struct{ db *gorm.DB }

func NewPlanRepo(db *gorm.DB) *PlanRepo { return &PlanRepo{db: db} }

func (r *PlanRepo) Create(ctx context.Context, p schema.Plan) error {
	return r.db.WithContext(ctx).Create(&p).Error
}
func (r *PlanRepo) CreateOnTx(ctx context.Context, tx *gorm.DB, p schema.Plan) error {
	return tx.WithContext(ctx).Create(&p).Error
}
func (r *PlanRepo) Get(ctx context.Context, id string) (schema.Plan, error) {
	var p schema.Plan
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	return p, err
}
func (r *PlanRepo) List(ctx context.Context, tenantID string) ([]schema.Plan, error) {
	var plans []schema.Plan
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&plans).Error
	return plans, err
}
func (r *PlanRepo) Update(ctx context.Context, p schema.Plan) error {
	return r.db.WithContext(ctx).Save(&p).Error
}
func (r *PlanRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.Plan{}, "id = ?", id).Error
}
func (r *PlanRepo) CountKeys(ctx context.Context, planID string) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&schema.APIKey{}).Where("plan_id = ?", planID).Count(&count).Error
	return int(count), err
}

// EnsureDefault seeds default plan templates if no plans exist for the tenant.
func (r *PlanRepo) EnsureDefault(ctx context.Context, tenantID string) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&schema.Plan{}).Where("tenant_id = ?", tenantID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaults := []schema.Plan{
		{
			ID:          "default",
			TenantID:    tenantID,
			Name:        "Default Plan",
			Description: "Standard plan with default access and no strict budget caps.",
			Period:      "monthly",
			AlertPct:    80,
			HardCutoff:  false,
		},
		{
			ID:          "starter",
			TenantID:    tenantID,
			Name:        "Starter Plan",
			Description: "Entry-level plan with $10/month spend limit.",
			LimitMicros: 10_000_000,
			Period:      "monthly",
			AlertPct:    80,
			HardCutoff:  true,
		},
		{
			ID:          "pro",
			TenantID:    tenantID,
			Name:        "Pro Plan",
			Description: "Professional plan with $100/month spend limit.",
			LimitMicros: 100_000_000,
			Period:      "monthly",
			AlertPct:    80,
			HardCutoff:  true,
		},
	}

	for _, p := range defaults {
		if err := r.db.WithContext(ctx).FirstOrCreate(&p, "id = ?", p.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

