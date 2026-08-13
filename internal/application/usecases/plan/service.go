// Package plan provides use cases for plan management.
package plan

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/dto"
	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Service provides plan management use cases.
type Service struct {
	repo  ports.PlanRepository
	clock ports.Clock
}

// NewService creates a new plan Service.
func NewService(repo ports.PlanRepository, clock ports.Clock) *Service {
	return &Service{repo: repo, clock: clock}
}

// CreatePlan creates a new plan.
func (s *Service) CreatePlan(ctx context.Context, req dto.CreatePlanRequest) (string, error) {
	return "", nil
}

// UpdatePlan modifies an existing plan.
func (s *Service) UpdatePlan(ctx context.Context, id string, req dto.UpdatePlanRequest) error {
	return nil
}

// DeletePlan removes a plan by ID.
func (s *Service) DeletePlan(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
