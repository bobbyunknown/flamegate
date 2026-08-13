package persistence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func TestExtensionRepo_CRUD(t *testing.T) {
	db := newPersistenceTestDB(t)
	ctx := context.Background()

	ext := schema.Extension{
		ID:           uuid.NewString(),
		TenantID:     "t1",
		Slug:         "kiro",
		Name:         "Kiro AI",
		Version:      "1.0.0",
		State:        "ACTIVE",
		Capabilities: `["chat","models"]`,
		Entrypoints:  `{"chat":"invoke","models":"list_models"}`,
	}

	// Create
	require.NoError(t, db.Extensions().Create(ctx, ext))

	// Get
	got, err := db.Extensions().Get(ctx, ext.ID)
	require.NoError(t, err)
	assert.Equal(t, "kiro", got.Slug)
	assert.Equal(t, "ACTIVE", got.State)

	// FindBySlug
	bySlug, err := db.Extensions().FindBySlug(ctx, "kiro")
	require.NoError(t, err)
	assert.Equal(t, ext.ID, bySlug.ID)

	// List
	list, err := db.Extensions().List(ctx, "t1")
	require.NoError(t, err)
	assert.Len(t, list, 1)

	// ListByState
	active, err := db.Extensions().ListByState(ctx, "ACTIVE")
	require.NoError(t, err)
	assert.Len(t, active, 1)

	// Update
	got.State = "DISABLED"
	require.NoError(t, db.Extensions().Update(ctx, got))
	updated, err := db.Extensions().Get(ctx, ext.ID)
	require.NoError(t, err)
	assert.Equal(t, "DISABLED", updated.State)

	// Delete
	require.NoError(t, db.Extensions().Delete(ctx, ext.ID))
	_, err = db.Extensions().Get(ctx, ext.ID)
	assert.Error(t, err)
}

func TestExtensionModelRepo_CRUD(t *testing.T) {
	db := newPersistenceTestDB(t)
	ctx := context.Background()

	// Create extension first
	ext := schema.Extension{
		ID:       uuid.NewString(),
		TenantID: "t1",
		Slug:     "kiro",
		Name:     "Kiro AI",
		Version:  "1.0.0",
		State:    "ACTIVE",
	}
	require.NoError(t, db.Extensions().Create(ctx, ext))

	model := schema.ExtensionModel{
		ID:          uuid.NewString(),
		ExtensionID: ext.ID,
		TenantID:    "t1",
		Slug:        "kiro-v1",
		DisplayName: "Kiro V1",
		Source:      "discovered",
	}

	// Create
	require.NoError(t, db.ExtensionModels().Create(ctx, model))

	// Get
	got, err := db.ExtensionModels().Get(ctx, model.ID)
	require.NoError(t, err)
	assert.Equal(t, "kiro-v1", got.Slug)

	// ListByExtension
	models, err := db.ExtensionModels().ListByExtension(ctx, ext.ID)
	require.NoError(t, err)
	assert.Len(t, models, 1)

	// ListBySource
	discovered, err := db.ExtensionModels().ListBySource(ctx, ext.ID, "discovered")
	require.NoError(t, err)
	assert.Len(t, discovered, 1)

	custom, err := db.ExtensionModels().ListBySource(ctx, ext.ID, "custom")
	require.NoError(t, err)
	assert.Len(t, custom, 0)

	// Update
	got.DisplayName = "Kiro V1 Updated"
	require.NoError(t, db.ExtensionModels().Update(ctx, got))
	updated, err := db.ExtensionModels().Get(ctx, model.ID)
	require.NoError(t, err)
	assert.Equal(t, "Kiro V1 Updated", updated.DisplayName)

	// Delete
	require.NoError(t, db.ExtensionModels().Delete(ctx, model.ID))
	_, err = db.ExtensionModels().Get(ctx, model.ID)
	assert.Error(t, err)
}

func TestExtensionModelRepo_DeleteBySource(t *testing.T) {
	db := newPersistenceTestDB(t)
	ctx := context.Background()

	ext := schema.Extension{
		ID:       uuid.NewString(),
		TenantID: "t1",
		Slug:     "kiro",
		Name:     "Kiro AI",
		Version:  "1.0.0",
		State:    "ACTIVE",
	}
	require.NoError(t, db.Extensions().Create(ctx, ext))

	require.NoError(t, db.ExtensionModels().Create(ctx, schema.ExtensionModel{
		ID: uuid.NewString(), ExtensionID: ext.ID, TenantID: "t1",
		Slug: "kiro-v1", Source: "discovered",
	}))
	require.NoError(t, db.ExtensionModels().Create(ctx, schema.ExtensionModel{
		ID: uuid.NewString(), ExtensionID: ext.ID, TenantID: "t1",
		Slug: "kiro-custom", Source: "custom",
	}))

	// Delete only discovered
	require.NoError(t, db.ExtensionModels().DeleteBySource(ctx, ext.ID, "discovered"))

	remaining, err := db.ExtensionModels().ListByExtension(ctx, ext.ID)
	require.NoError(t, err)
	assert.Len(t, remaining, 1)
	assert.Equal(t, "custom", remaining[0].Source)
}

func TestExtensionModelRepo_DeleteByExtension(t *testing.T) {
	db := newPersistenceTestDB(t)
	ctx := context.Background()

	ext := schema.Extension{
		ID:       uuid.NewString(),
		TenantID: "t1",
		Slug:     "kiro",
		Name:     "Kiro AI",
		Version:  "1.0.0",
		State:    "ACTIVE",
	}
	require.NoError(t, db.Extensions().Create(ctx, ext))

	require.NoError(t, db.ExtensionModels().Create(ctx, schema.ExtensionModel{
		ID: uuid.NewString(), ExtensionID: ext.ID, TenantID: "t1",
		Slug: "kiro-v1", Source: "discovered",
	}))
	require.NoError(t, db.ExtensionModels().Create(ctx, schema.ExtensionModel{
		ID: uuid.NewString(), ExtensionID: ext.ID, TenantID: "t1",
		Slug: "kiro-custom", Source: "custom",
	}))

	require.NoError(t, db.ExtensionModels().DeleteByExtension(ctx, ext.ID))

	remaining, err := db.ExtensionModels().ListByExtension(ctx, ext.ID)
	require.NoError(t, err)
	assert.Len(t, remaining, 0)
}

func TestExtensionModelRepo_ListByTenant(t *testing.T) {
	db := newPersistenceTestDB(t)
	ctx := context.Background()

	// Two extensions on different tenants.
	ext1 := schema.Extension{
		ID: uuid.NewString(), TenantID: "t1", Slug: "alpha", Name: "Alpha", Version: "1.0", State: "ACTIVE",
	}
	ext2 := schema.Extension{
		ID: uuid.NewString(), TenantID: "t2", Slug: "beta", Name: "Beta", Version: "1.0", State: "ACTIVE",
	}
	require.NoError(t, db.Extensions().Create(ctx, ext1))
	require.NoError(t, db.Extensions().Create(ctx, ext2))

	// Models on tenant t1.
	require.NoError(t, db.ExtensionModels().Create(ctx, schema.ExtensionModel{
		ID: uuid.NewString(), ExtensionID: ext1.ID, TenantID: "t1", Slug: "alpha-m1", DisplayName: "Alpha M1", Source: "discovered",
	}))
	require.NoError(t, db.ExtensionModels().Create(ctx, schema.ExtensionModel{
		ID: uuid.NewString(), ExtensionID: ext1.ID, TenantID: "t1", Slug: "alpha-m2", DisplayName: "Alpha M2", Source: "custom",
	}))
	// Model on tenant t2.
	require.NoError(t, db.ExtensionModels().Create(ctx, schema.ExtensionModel{
		ID: uuid.NewString(), ExtensionID: ext2.ID, TenantID: "t2", Slug: "beta-m1", DisplayName: "Beta M1", Source: "discovered",
	}))

	// ListByTenant t1 — should return 2.
	t1Models, err := db.ExtensionModels().ListByTenant(ctx, "t1")
	require.NoError(t, err)
	assert.Len(t, t1Models, 2)

	// ListByTenant t2 — should return 1.
	t2Models, err := db.ExtensionModels().ListByTenant(ctx, "t2")
	require.NoError(t, err)
	assert.Len(t, t2Models, 1)

	// ListByTenant nonexistent — empty.
	empty, err := db.ExtensionModels().ListByTenant(ctx, "t999")
	require.NoError(t, err)
	assert.Empty(t, empty)
}
