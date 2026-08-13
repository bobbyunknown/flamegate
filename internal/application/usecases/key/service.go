// Package key provides use cases for API key management.
package key

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/dto"
	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Service provides key management use cases.
type Service struct {
	repo  ports.APIKeyRepository
	clock ports.Clock
}

// NewService creates a new key Service.
func NewService(repo ports.APIKeyRepository, clock ports.Clock) *Service {
	return &Service{repo: repo, clock: clock}
}

// CreateKey creates a new API key and returns the plaintext (shown once).
func (s *Service) CreateKey(ctx context.Context, req dto.CreateKeyRequest) (dto.CreateKeyResult, error) {
	// TODO: generate key, hash, store
	return dto.CreateKeyResult{}, nil
}

// RotateKey revokes the old key and creates a new one.
func (s *Service) RotateKey(ctx context.Context, id string) (dto.CreateKeyResult, error) {
	return dto.CreateKeyResult{}, nil
}

// DeleteKey removes a key by ID.
func (s *Service) DeleteKey(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
