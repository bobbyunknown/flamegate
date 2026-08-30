package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseRawCatalog unmarshals models.dev JSON data and constructs two lookup indices:
// 1. byProviderModel: keyed by "<provider>/<model_id>" (e.g. "openai/gpt-4o")
// 2. canonicalIndex: keyed by base model slug (e.g. "gpt-4o")
func ParseRawCatalog(data []byte) (map[string]*ModelSpec, map[string]*ModelSpec, error) {
	var raw RawCatalog
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, fmt.Errorf("unmarshal raw catalog: %w", err)
	}

	byProviderModel := make(map[string]*ModelSpec, len(raw)*10)
	canonicalIndex := make(map[string]*ModelSpec, len(raw)*10)

	for providerID, provider := range raw {
		normProvider := strings.ToLower(strings.TrimSpace(providerID))
		for modelID, model := range provider.Models {
			normModelID := strings.TrimSpace(modelID)
			spec := &ModelSpec{
				ID:          normModelID,
				Name:        model.Name,
				Provider:    normProvider,
				Description: model.Description,
				Reasoning:   model.Reasoning,
				ToolCall:    model.ToolCall,
				Modalities: ModelModalities{
					Input:  []string{},
					Output: []string{},
				},
			}

			if model.Limit != nil {
				spec.Limits = ModelLimits{
					Context: model.Limit.Context,
					Output:  model.Limit.Output,
				}
			}

			if model.Cost != nil {
				spec.Cost = ModelCost{
					Input:      model.Cost.Input,
					Output:     model.Cost.Output,
					CacheRead:  model.Cost.CacheRead,
					CacheWrite: model.Cost.CacheWrite,
					Reasoning:  model.Cost.Reasoning,
				}
			}

			if model.Modalities != nil {
				if len(model.Modalities.Input) > 0 {
					spec.Modalities.Input = model.Modalities.Input
				}
				if len(model.Modalities.Output) > 0 {
					spec.Modalities.Output = model.Modalities.Output
				}
			}

			key := normProvider + "/" + normModelID
			byProviderModel[key] = spec

			baseSlug := ExtractBaseModelSlug(normModelID)
			if baseSlug != "" {
				if _, exists := canonicalIndex[baseSlug]; !exists {
					canonicalIndex[baseSlug] = spec
				}
			}
		}
	}

	return byProviderModel, canonicalIndex, nil
}
