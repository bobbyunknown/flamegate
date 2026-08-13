// Package chain provides use cases for routing chain management.
package chain

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Service provides chain management use cases.
type Service struct {
	repo  ports.ChainRepository
	clock ports.Clock
}

// NewService creates a new chain Service.
func NewService(repo ports.ChainRepository, clock ports.Clock) *Service {
	return &Service{repo: repo, clock: clock}
}

// CreateChain creates a new routing chain.
func (s *Service) CreateChain(ctx context.Context, tenantID, name, strategy string) (string, error) {
	return "", nil
}

// DeleteChain removes a chain by ID.
func (s *Service) DeleteChain(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
