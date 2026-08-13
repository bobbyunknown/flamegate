package admin

import (
	"context"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/identity"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
)

func TestHumaCreateKey_WithPlan(t *testing.T) {
	db, err := persistence.OpenDB("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	require.NoError(t, db.EnsureDefault())
	t.Cleanup(func() { _ = db.Close() })

	idService := identity.New(db.APIKeys())

	handler := &Handler{
		db:       db,
		identity: idService,
		budgets:  db.Budgets(),
		log:      logrus.StandardLogger(),
	}

	// 1. With Plan
	input := &CreateKeyInput{
		Body: CreateKeyBody{
			Name:   "Plan Key",
			PlanID: "default",
		},
	}
	out, err := handler.HumaCreateKey(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, "Plan Key", out.Body.Name)
	require.Equal(t, "default", out.Body.PlanID)

	// 2. With Plan + Budget Overrides
	limitUSD := 50.0
	limitTokens := int64(10_000_000)
	input2 := &CreateKeyInput{
		Body: CreateKeyBody{
			Name:              "Override Key",
			PlanID:            "default",
			BudgetLimitUSD:    &limitUSD,
			BudgetLimitTokens: &limitTokens,
			AllowedModels:     []string{"gpt-4o"},
		},
	}
	out2, err := handler.HumaCreateKey(context.Background(), input2)
	require.NoError(t, err)
	require.NotNil(t, out2)
	require.Equal(t, "Override Key", out2.Body.Name)
	require.Equal(t, []string{"gpt-4o"}, out2.Body.AllowedModels)
}
