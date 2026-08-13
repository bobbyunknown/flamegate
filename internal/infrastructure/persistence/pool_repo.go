package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type ProxyPoolRepo struct{ db *gorm.DB }

func NewProxyPoolRepo(db *gorm.DB) *ProxyPoolRepo { return &ProxyPoolRepo{db: db} }

func (r *ProxyPoolRepo) Get(ctx context.Context, id string) (schema.ProxyPool, error) {
	var p schema.ProxyPool
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	return p, err
}
func (r *ProxyPoolRepo) List(ctx context.Context) ([]schema.ProxyPool, error) {
	var pools []schema.ProxyPool
	err := r.db.WithContext(ctx).Find(&pools).Error
	return pools, err
}
func (r *ProxyPoolRepo) Create(ctx context.Context, p schema.ProxyPool) error {
	return r.db.WithContext(ctx).Create(&p).Error
}
func (r *ProxyPoolRepo) Update(ctx context.Context, p schema.ProxyPool) error {
	return r.db.WithContext(ctx).Save(&p).Error
}
func (r *ProxyPoolRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&schema.ProxyPool{}, "id = ?", id).Error
}
