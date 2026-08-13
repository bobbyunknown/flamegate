// Package budget provides use cases for budget management.
package budget

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Service provides budget management use cases.
type Service struct {
	repo  ports.BudgetRepository
	clock ports.Clock
}

// NewService creates a new budget Service.
func NewService(repo ports.BudgetRepository, clock ports.Clock) *Service {
	return &Service{repo: repo, clock: clock}
}

// CreateBudget creates a new budget.
func (s *Service) CreateBudget(ctx context.Context, scope, scopeID string, limitMicros, limitTokens int64, period string) (string, error) {
	return "", nil
}

// DeleteBudget removes a budget by ID.
func (s *Service) DeleteBudget(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
