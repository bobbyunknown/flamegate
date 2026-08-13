// Package chat provides the chat completion use case.
package chat

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Service wraps the existing pipeline for chat completions.
type Service struct {
	accounts ports.AccountRepository
	usage    ports.UsageRepository
}

// NewService creates a new chat Service.
func NewService(accounts ports.AccountRepository, usage ports.UsageRepository) *Service {
	return &Service{accounts: accounts, usage: usage}
}

// ProcessChat handles a non-streaming chat request.
func (s *Service) ProcessChat(ctx context.Context, req interface{}) (interface{}, error) {
	// TODO: wrap existing pipeline.Process
	return nil, nil
}
