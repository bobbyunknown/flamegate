package persistence

import (
	"context"

	"gorm.io/gorm"
)

// UnitOfWork implements ports.UnitOfWork using GORM transactions.
type UnitOfWork struct {
	db *gorm.DB
}

// NewUnitOfWork creates a new GORM-backed Unit of Work.
func NewUnitOfWork(db *gorm.DB) *UnitOfWork {
	return &UnitOfWork{db: db}
}

// Run executes fn inside a database transaction.
func (u *UnitOfWork) Run(ctx context.Context, fn func(ctx context.Context) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx)
	})
}

// DB returns the underlying GORM database (for bootstrap wiring).
func (u *UnitOfWork) DB() *gorm.DB { return u.db }
