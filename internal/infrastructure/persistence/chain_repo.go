package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type ChainRepo struct{ db *gorm.DB }

func NewChainRepo(db *gorm.DB) *ChainRepo { return &ChainRepo{db: db} }

func (r *ChainRepo) Create(ctx context.Context, c schema.Chain) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
		for i := range c.Steps {
			if c.Steps[i].ID == "" {
				c.Steps[i].ID = uuid.NewString()
			}
			c.Steps[i].ChainID = c.ID
			c.Steps[i].CreatedAt = time.Now()
			if err := tx.Create(&c.Steps[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *ChainRepo) Get(ctx context.Context, id string) (schema.Chain, error) {
	var c schema.Chain
	err := r.db.WithContext(ctx).First(&c, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return schema.Chain{}, schema.ErrNotFound
		}
		return c, err
	}
	var steps []schema.ChainStep
	if err := r.db.WithContext(ctx).Where("chain_id = ?", id).Order("position ASC").Find(&steps).Error; err != nil {
		return c, err
	}
	c.Steps = steps
	return c, nil
}
func (r *ChainRepo) ListByTenant(ctx context.Context, tenantID string) ([]schema.Chain, error) {
	var chains []schema.Chain
	if err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&chains).Error; err != nil {
		return nil, err
	}
	for i := range chains {
		var steps []schema.ChainStep
		r.db.WithContext(ctx).Where("chain_id = ?", chains[i].ID).Order("position ASC").Find(&steps)
		chains[i].Steps = steps
	}
	return chains, nil
}
func (r *ChainRepo) Update(ctx context.Context, c schema.Chain) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&c).Error; err != nil {
			return err
		}
		if err := tx.Where("chain_id = ?", c.ID).Delete(&schema.ChainStep{}).Error; err != nil {
			return err
		}
		for i := range c.Steps {
			c.Steps[i].ChainID = c.ID
			c.Steps[i].CreatedAt = time.Now()
			if err := tx.Create(&c.Steps[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *ChainRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("chain_id = ?", id).Delete(&schema.ChainStep{}).Error; err != nil {
			return err
		}
		return tx.Delete(&schema.Chain{}, "id = ?", id).Error
	})
}
