package admin

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
)

func TestProviderAccountMetadataSpecialProviders(t *testing.T) {
	custom, ok := connectors.SpecByID("custom-openai")
	require.True(t, ok)
	_, err := providerAccountMetadata(custom, providerMetadataInput{})
	require.Error(t, err)
	meta, err := providerAccountMetadata(custom, providerMetadataInput{BaseURL: "https://llm.example.com/v1"})
	require.NoError(t, err)
	require.Equal(t, "https://llm.example.com/v1", meta["base_url"])
}

func TestParseRefreshFlag(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on", " True "} {
		if !parseRefreshFlag(v) {
			t.Fatalf("parseRefreshFlag(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "refresh"} {
		if parseRefreshFlag(v) {
			t.Fatalf("parseRefreshFlag(%q) = true, want false", v)
		}
	}
}
