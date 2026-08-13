package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
