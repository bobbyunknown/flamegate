package persistence

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

type ResourceRepo struct{ db *gorm.DB }

func NewResourceRepo(db *gorm.DB) *ResourceRepo { return &ResourceRepo{db: db} }

func (r *ResourceRepo) InsertResourceSample(ctx context.Context, s schema.ResourceSample) error {
	return r.db.WithContext(ctx).Create(&s).Error
}
func (r *ResourceRepo) ResourceBuckets(ctx context.Context, since time.Time, interval time.Duration) ([]schema.ResourceBucket, error) {
	var results []schema.ResourceBucket
	err := r.db.WithContext(ctx).Where("created_at >= ?", since).Find(&results).Error
	return results, err
}
func (r *ResourceRepo) PruneResourceSamples(ctx context.Context, maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge)
	return r.db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&schema.ResourceSample{}).Error
}
