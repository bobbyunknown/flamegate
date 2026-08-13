package persistence

import (
	"context"
	"errors"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"gorm.io/gorm"
)

type UserRepo struct{ db *gorm.DB }

func NewUserRepo(db *gorm.DB) *UserRepo { return &UserRepo{db: db} }

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (schema.User, error) {
	var u schema.User
	err := r.db.WithContext(ctx).First(&u, "username = ?", username).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return schema.User{}, schema.ErrNotFound
		}
		return schema.User{}, err
	}
	return u, nil
}

func (r *UserRepo) Create(ctx context.Context, u *schema.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id, hash string) error {
	return r.db.WithContext(ctx).
		Model(&schema.User{}).
		Where("id = ?", id).
		Update("password_hash", hash).Error
}
