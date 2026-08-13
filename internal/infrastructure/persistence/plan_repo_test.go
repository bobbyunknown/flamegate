package persistence

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func TestPlanRepo_EnsureDefault(t *testing.T) {
	db := newPersistenceTestDB(t)
	ctx := context.Background()

	// Initial EnsureDefault should seed default plans.
	err := db.Plans().EnsureDefault(ctx, schema.DefaultTenantID)
	require.NoError(t, err)

	plans, err := db.Plans().List(ctx, schema.DefaultTenantID)
	require.NoError(t, err)
	assert.Len(t, plans, 3)

	// Verify plan IDs exist.
	planIDs := make(map[string]bool)
	for _, p := range plans {
		planIDs[p.ID] = true
	}
	assert.True(t, planIDs["default"])
	assert.True(t, planIDs["starter"])
	assert.True(t, planIDs["pro"])

	// Running EnsureDefault again should be idempotent.
	err = db.Plans().EnsureDefault(ctx, schema.DefaultTenantID)
	require.NoError(t, err)

	plansAfter, err := db.Plans().List(ctx, schema.DefaultTenantID)
	require.NoError(t, err)
	assert.Len(t, plansAfter, 3)
}
