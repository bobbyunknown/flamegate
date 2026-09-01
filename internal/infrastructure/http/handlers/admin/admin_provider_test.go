package admin

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
)

func newAdminTestDB(t *testing.T) *persistence.DB {
	t.Helper()
	db, err := persistence.OpenDB("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	return db
}

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

func TestHumaProviderModels_EmptySliceNotNull(t *testing.T) {
	// Test that an unknown/empty provider models call returns empty slice not nil
	h := &Handler{}
	connectors.RegisterExtensionSpec(connectors.ProviderSpec{
		ID:          "test-ext",
		DisplayName: "Test Ext",
	})
	defer connectors.UnregisterExtensionSpec("test-ext")

	out, err := h.HumaProviderModels(context.Background(), &ProviderModelsInput{
		ID: "test-ext",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.NotNil(t, out.Body.Models)
	require.Equal(t, 0, len(out.Body.Models))
}

func TestSyncExtensionModels_Success(t *testing.T) {
	db := newAdminTestDB(t)
	ctx := context.Background()

	// Register extension in DB and spec catalog
	connectors.RegisterExtensionSpec(connectors.ProviderSpec{
		ID:          "cline",
		DisplayName: "Cline",
	})
	defer connectors.UnregisterExtensionSpec("cline")

	ext := schema.Extension{
		ID:           "cline",
		TenantID:     "default",
		Slug:         "cline",
		Name:         "Cline",
		Version:      "0.1.0",
		State:        "ACTIVE",
		Capabilities: `["chat","models"]`,
		Entrypoints:  `{"chat":"invoke","models":"list_models"}`,
	}
	require.NoError(t, db.Extensions().Create(ctx, ext))

	// Setup WASM Engine
	wasmBytes, err := os.ReadFile("../../../../../flamegate-ext/cline/dist/cline.wasm")
	if err != nil {
		t.Skip("skipping wasm test: cline.wasm not found")
	}

	wasmEngine := wasm.NewEngine(config.WASMConfig{
		MaxMemoryMB:    16,
		MaxInst:        4,
		DefaultTimeout: 10 * time.Second,
	}, nil, db.Accounts(), nil)
	defer wasmEngine.Close()

	require.NoError(t, wasmEngine.Compile(ctx, "cline", wasmBytes, wasm.ExtensionConfig{
		Slug:        "cline",
		Timeout:     10 * time.Second,
		Entrypoints: map[string]string{"chat": "invoke", "models": "list_models"},
	}))

	h := &Handler{
		db:         db,
		wasmEngine: wasmEngine,
		log:        logrus.New(),
	}

	n, err := h.syncExtensionModels(ctx, ext)
	require.NoError(t, err)
	require.Greater(t, n, 0)
	t.Logf("Successfully synced %d models into database", n)

	// Verify they are in extension_models table
	ems, err := db.ExtensionModels().ListByExtension(ctx, "cline")
	require.NoError(t, err)
	require.Equal(t, n, len(ems))
	t.Logf("First model in DB: %+v", ems[0])

	// Verify HumaProviderModels returns them
	out, err := h.HumaProviderModels(ctx, &ProviderModelsInput{
		ID: "cline",
	})
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, n, len(out.Body.Models))
}
