// Package settings provides use cases for settings management.
package settings

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Service provides settings management use cases.
type Service struct {
	repo ports.SettingsRepository
}

// NewService creates a new settings Service.
func NewService(repo ports.SettingsRepository) *Service {
	return &Service{repo: repo}
}

// Get retrieves a setting value by key.
func (s *Service) Get(ctx context.Context, key string) (string, error) {
	return s.repo.Get(ctx, key)
}

// Set writes a setting value.
func (s *Service) Set(ctx context.Context, key, value string) error {
	return s.repo.Set(ctx, key, value)
}
