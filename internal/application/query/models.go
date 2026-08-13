package query

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// ModelInfo represents a model for API responses.
type ModelInfo struct {
	ID            string `json:"id"`
	ProviderID    string `json:"provider_id"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	ContextWindow int    `json:"context_window,omitempty"`
	MaxOutput     int    `json:"max_output,omitempty"`
}

// ModelQuery provides read-only model queries.
type ModelQuery struct {
	connectorRepo ports.CustomProviderRepository
}

// NewModelQuery creates a new ModelQuery.
func NewModelQuery(repo ports.CustomProviderRepository) *ModelQuery {
	return &ModelQuery{connectorRepo: repo}
}

// ListModels returns all available models.
func (q *ModelQuery) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return nil, nil
}

// GetModelInfo returns details for a specific model.
func (q *ModelQuery) GetModelInfo(ctx context.Context, id string) (*ModelInfo, error) {
	return nil, nil
}
