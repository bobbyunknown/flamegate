// Package account provides use cases for provider account management.
package account

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/dto"
	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Service provides account management use cases.
type Service struct {
	repo  ports.AccountRepository
	clock ports.Clock
}

// NewService creates a new account Service.
func NewService(repo ports.AccountRepository, clock ports.Clock) *Service {
	return &Service{repo: repo, clock: clock}
}

// CreateAccount registers a new provider account.
func (s *Service) CreateAccount(ctx context.Context, req dto.CreateAccountRequest) (string, error) {
	return "", nil
}

// UpdateAccount modifies an existing account.
func (s *Service) UpdateAccount(ctx context.Context, id string, req dto.UpdateAccountRequest) error {
	return nil
}

// DeleteAccount removes an account by ID.
func (s *Service) DeleteAccount(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// TestAccount validates an account connection.
func (s *Service) TestAccount(ctx context.Context, id string) (dto.TestAccountResult, error) {
	return dto.TestAccountResult{}, nil
}
