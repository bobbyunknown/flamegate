package admin

import (
	"context"
	"encoding/json"
)

// ---- disabled models --------------------------------------------------------

const disabledModelsPrefix = "disabled_models_" // + provider alias

func (s *Handler) loadDisabledModels(ctx context.Context, provider string) []string {
	if s.settings == nil {
		return nil
	}
	raw, err := s.settings.Get(ctx, disabledModelsPrefix+provider)
	if err != nil || raw == "" {
		return nil //nolint:nilerr // best-effort loader
	}
	var ids []string
	if err := json.Unmarshal([]byte(raw), &ids); err != nil {
		return nil //nolint:nilerr // best-effort loader
	}
	return ids
}

func (s *Handler) saveDisabledModels(ctx context.Context, provider string, ids []string) error {
	if s.settings == nil {
		return nil
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return s.settings.Set(ctx, disabledModelsPrefix+provider, string(raw))
}

// ---- console SSE stream -----------------------------------------------------
