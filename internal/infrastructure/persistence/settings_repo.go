package persistence

import (
	"context"
	"errors"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"

	"gorm.io/gorm"
)

// Settings is a key/value configuration row (no store dependency).
type Settings struct {
	Key   string `gorm:"primaryKey;type:varchar(255)"`
	Value string `gorm:"type:text"`
}

type SettingsRepo struct{ db *gorm.DB }

func NewSettingsRepo(db *gorm.DB) *SettingsRepo { return &SettingsRepo{db: db} }

func (r *SettingsRepo) Get(ctx context.Context, key string) (string, error) {
	var s Settings
	err := r.db.WithContext(ctx).First(&s, "key = ?", key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", schema.ErrNotFound
		}
		return "", err
	}
	return s.Value, nil
}

func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	return r.db.WithContext(ctx).Save(&Settings{Key: key, Value: value}).Error
}
