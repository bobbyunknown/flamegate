package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/capability"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func TestExtensionModelEntries_CustomWinsOverDiscovered(t *testing.T) {
	// Custom and discovered with the same model id — custom must appear first
	// so the seen-map dedup gives it priority.
	input := []schema.ExtensionModel{
		{
			ID:          "my-provider/m1",
			ExtensionID: "ext-id-1",
			Slug:        "m1",
			DisplayName: "My Provider (discovered)",
			Source:      "discovered",
		},
		{
			ID:          "my-provider/m1",
			ExtensionID: "ext-id-1",
			Slug:        "m1",
			DisplayName: "My Provider Custom",
			Source:      "custom",
		},
	}

	entries := extensionModelEntries(input)
	require.Len(t, entries, 2)

	// Custom must come first so dedup in listModels picks it.
	assert.Equal(t, "custom", entries[0].Source)
	assert.Equal(t, "My Provider Custom", entries[0].Name)
	assert.Equal(t, "my-provider/m1", entries[0].ID)
	assert.Equal(t, "my-provider", entries[0].Provider)
	assert.Equal(t, "discovered", entries[1].Source)
}

func TestExtensionModelEntries_DiscoveredOnly(t *testing.T) {
	input := []schema.ExtensionModel{
		{
			ID:          "kiro/kiro-ext",
			ExtensionID: "ext-id-1",
			Slug:        "kiro-ext",
			DisplayName: "Kiro v1",
			Source:      "discovered",
		},
	}

	entries := extensionModelEntries(input)
	require.Len(t, entries, 1)
	assert.Equal(t, "discovered", entries[0].Source)
	assert.Equal(t, "kiro/kiro-ext", entries[0].ID)
	assert.Equal(t, "kiro", entries[0].OwnedBy)
	assert.Equal(t, "kiro", entries[0].Provider)
	assert.Equal(t, "model", entries[0].Object)
	assert.Equal(t, "llm", entries[0].Kind)
}

func TestExtensionModelEntries_Empty(t *testing.T) {
	entries := extensionModelEntries(nil)
	assert.Empty(t, entries)

	entries = extensionModelEntries([]schema.ExtensionModel{})
	assert.Empty(t, entries)
}

func TestExtensionModelEntries_MultipleSlugs(t *testing.T) {
	input := []schema.ExtensionModel{
		{
			ID:          "provider-a/m-a",
			ExtensionID: "ext-id-1",
			Slug:        "m-a",
			DisplayName: "Model A",
			Source:      "discovered",
		},
		{
			ID:          "provider-b/m-b",
			ExtensionID: "ext-id-2",
			Slug:        "m-b",
			DisplayName: "Model B",
			Source:      "discovered",
		},
	}

	entries := extensionModelEntries(input)
	require.Len(t, entries, 2)
	assert.Equal(t, "provider-a/m-a", entries[0].ID)
	assert.Equal(t, "provider-b/m-b", entries[1].ID)
}

func TestExtensionModelEntries_SourceField(t *testing.T) {
	input := []schema.ExtensionModel{
		{
			ID:          "test/m1",
			ExtensionID: "ext-id-1",
			Slug:        "m1",
			DisplayName: "Test",
			Source:      "custom",
		},
	}

	entries := extensionModelEntries(input)
	require.Len(t, entries, 1)
	assert.Equal(t, "custom", entries[0].Source)
	assert.Equal(t, "test", entries[0].Provider)
	assert.Equal(t, "test/m1", entries[0].ID)
}

func TestEnrichModelEntry_WithCatalogAndCapabilities(t *testing.T) {
	catSvc := catalog.NewService(catalog.Config{})
	err := catSvc.LoadFromBytes([]byte(`{
		"google": {
			"id": "google",
			"name": "Google",
			"models": {
				"gemini-2.5-flash": {
					"id": "gemini-2.5-flash",
					"name": "Gemini 2.5 Flash",
					"tool_call": true,
					"modalities": {
						"input": ["text", "image", "audio", "video", "pdf"],
						"output": ["text"]
					},
					"cost": {
						"input": 0.3,
						"output": 2.5,
						"cache_read": 0.03
					},
					"limit": {
						"context": 1048576,
						"output": 65536
					}
				}
			}
		}
	}`))
	require.NoError(t, err)

	capability.SetCatalogSource(catSvc)
	defer capability.SetCatalogSource(nil)

	h := &Handler{
		catalog: catSvc,
	}

	entry := modelEntry{
		ID:       "antigravity/gemini-2.5-flash",
		Provider: "antigravity",
		Kind:     "llm",
		Name:     "Gemini 2.5 Flash",
	}

	h.enrichModelEntry(&entry)

	assert.Equal(t, 1048576, entry.ContextWindow)
	assert.Equal(t, 65536, entry.MaxOutputTokens)
	assert.Contains(t, entry.InputModalities, "image")
	assert.Contains(t, entry.InputModalities, "pdf")
	require.NotNil(t, entry.Pricing)
	assert.Equal(t, 0.3, entry.Pricing.InputPerM)
	assert.Equal(t, 2.5, entry.Pricing.OutputPerM)
}

