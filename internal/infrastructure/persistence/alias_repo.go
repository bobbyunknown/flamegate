package persistence

import (
	"context"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type AliasRepo struct{ db *gorm.DB }

func NewAliasRepo(db *gorm.DB) *AliasRepo { return &AliasRepo{db: db} }

func (r *AliasRepo) List(ctx context.Context) ([]schema.ModelAlias, error) {
	var aliases []schema.ModelAlias
	err := r.db.WithContext(ctx).Order("alias").Find(&aliases).Error
	return aliases, err
}
func (r *AliasRepo) Get(ctx context.Context, alias string) (schema.ModelAlias, error) {
	var a schema.ModelAlias
	err := r.db.WithContext(ctx).First(&a, "alias = ?", alias).Error
	return a, err
}
func (r *AliasRepo) Set(ctx context.Context, alias, target string) error {
	return r.db.WithContext(ctx).Save(&schema.ModelAlias{Alias: alias, Target: target}).Error
}
func (r *AliasRepo) Delete(ctx context.Context, alias string) error {
	return r.db.WithContext(ctx).Delete(&schema.ModelAlias{}, "alias = ?", alias).Error
}
