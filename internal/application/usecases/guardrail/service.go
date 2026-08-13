// Package guardrail provides use cases for guardrail management.
package guardrail

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Service provides guardrail management use cases.
type Service struct {
	repo  ports.GuardrailRepository
	clock ports.Clock
}

// NewService creates a new guardrail Service.
func NewService(repo ports.GuardrailRepository, clock ports.Clock) *Service {
	return &Service{repo: repo, clock: clock}
}

// ListPolicies lists all guardrail policies for a tenant.
func (s *Service) ListPolicies(ctx context.Context, tenantID, scope string) (interface{}, error) {
	return nil, nil
}

// DeletePolicy removes a policy by ID.
func (s *Service) DeletePolicy(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}
