package dispatch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/crypto"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

type fakeConnectorSource struct {
	conn core.Connector
}

func (s fakeConnectorSource) Get(provider string) (core.Connector, error) {
	return s.conn, nil
}

type fakeConnector struct{}

func (fakeConnector) ID() string            { return "openai" }
func (fakeConnector) Dialect() core.Dialect { return core.DialectOpenAI }
func (fakeConnector) Chat(context.Context, *core.ChatRequest, core.Credentials) (*core.ChatResponse, error) {
	return nil, nil
}
func (fakeConnector) Stream(context.Context, *core.ChatRequest, core.Credentials, core.StreamConfig) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk)
	close(ch)
	return ch, nil
}

func newDispatchTest(t *testing.T, accounts ...schema.Account) (*Dispatcher, *persistence.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := persistence.OpenDB("sqlite", ":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate())
	require.NoError(t, db.EnsureDefault())
	t.Cleanup(func() { _ = db.Close() })

	mk, err := crypto.GenerateMasterKey()
	require.NoError(t, err)
	sealer, err := crypto.NewSealer(mk)
	require.NoError(t, err)
	v := vault.New(sealer)

	for i := range accounts {
		require.NoError(t, v.Seal(&accounts[i], vault.NewSecret{APIKey: "sk-test"}))
		require.NoError(t, db.Accounts().Create(ctx, accounts[i]))
	}

	d := New(fakeConnectorSource{conn: fakeConnector{}}, db.Accounts(), v)
	d.SetRoutingSource(db.Routing())
	return d, db
}

func testAccount(id string, priority int) schema.Account {
	now := time.Now()
	return schema.Account{
		ID:        id,
		TenantID:  schema.DefaultTenantID,
		Provider:  "openai",
		Label:     id,
		AuthKind:  string(schema.AuthAPIKey),
		Priority:  priority,
		Metadata:  "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestPlanWith_AccountRoundRobinRotatesProviderAccounts(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
		testAccount("acc-3", 30),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{AccountStrategy: StrategyRoundRobin}

	var got []string
	for i := 0; i < 4; i++ {
		attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		require.NotEmpty(t, attempts)
		got = append(got, attempts[0].Account.ID)
	}

	require.Equal(t, []string{"acc-1", "acc-2", "acc-3", "acc-1"}, got)
}

func TestPlanWith_AccountRoundRobinHonorsStickyLimit(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{AccountStrategy: StrategyRoundRobin, AccountStickyLimit: 2}

	var got []string
	for i := 0; i < 5; i++ {
		attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		require.NotEmpty(t, attempts)
		got = append(got, attempts[0].Account.ID)
	}

	require.Equal(t, []string{"acc-1", "acc-1", "acc-2", "acc-2", "acc-1"}, got)
}

func TestPlanWith_SmartRoundRobinPinsAffinityKey(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
		testAccount("acc-3", 30),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{AccountStrategy: StrategySmartRoundRobin, AccountAffinityKey: "thread-a"}

	attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
	require.NoError(t, err)
	require.Equal(t, "acc-1", attempts[0].Account.ID)

	attempts, err = d.PlanWith(ctx, schema.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
	require.NoError(t, err)
	require.Equal(t, "acc-1", attempts[0].Account.ID)

	opts.AccountAffinityKey = "thread-b"
	attempts, err = d.PlanWith(ctx, schema.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
	require.NoError(t, err)
	require.Equal(t, "acc-2", attempts[0].Account.ID)

	opts.AccountAffinityKey = "thread-a"
	attempts, err = d.PlanWith(ctx, schema.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
	require.NoError(t, err)
	require.Equal(t, "acc-1", attempts[0].Account.ID)
}

func TestPlanWith_ProviderAccountStrategyOverride(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{
		AccountStrategy: StrategyFallback,
		ProviderAccountStrategies: map[string]AccountRoutingOptions{
			"openai": {Strategy: StrategyRoundRobin},
		},
	}

	var got []string
	for i := 0; i < 3; i++ {
		attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		got = append(got, attempts[0].Account.ID)
	}

	require.Equal(t, []string{"acc-1", "acc-2", "acc-1"}, got)
}

func TestPlanWith_TargetRoundRobinRotatesComboTargets(t *testing.T) {
	ctx := context.Background()
	d, db := newDispatchTest(t, testAccount("acc-1", 10))
	now := time.Now()
	require.NoError(t, db.Chains().Create(ctx, schema.Chain{
		ID:        "chain-1",
		TenantID:  schema.DefaultTenantID,
		Name:      "combo",
		Strategy:  string(StrategyRoundRobin),
		CreatedAt: now,
		UpdatedAt: now,
	}))

	targets := []Target{
		{Provider: "openai", Model: "gpt-4o"},
		{Provider: "openai", Model: "gpt-5"},
	}
	opts := PlanOptions{Strategy: StrategyRoundRobin, ChainID: "chain-1"}

	var got []string
	for i := 0; i < 3; i++ {
		attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		got = append(got, attempts[0].Target.Model)
	}

	require.Equal(t, []string{"gpt-4o", "gpt-5", "gpt-4o"}, got)
}

func TestPlanWith_CustomOpenAISkipsCapabilityGuard(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t, testAccountWithProvider("acc-custom", "custom-openai", 10))

	// The built-in "custom-openai" generic gateway has unknown upstream
	// capabilities, so the guard must be relaxed for it too.
	required := core.NewCapabilitySet(core.CapVision)
	targets := []Target{{Provider: "custom-openai", Model: "glm-5.2"}}

	attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, required, PlanOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, attempts)
	require.Equal(t, "custom-openai", attempts[0].Target.Provider)
}

func TestPlanWith_CustomProviderSkipsCapabilityGuard(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t, testAccountWithProvider("acc-custom", "custom-openai-bandel", 10))

	// glm-5.2 on a custom provider lacks vision in its resolved profile.
	// A vision-required request would normally be rejected, but custom
	// providers bypass the guard because their upstream capabilities are
	// unknown — the pipeline soft-degrades modalities instead.
	required := core.NewCapabilitySet(core.CapVision)
	targets := []Target{{Provider: "custom-openai-bandel", Model: "glm-5.2"}}

	attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, required, PlanOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, attempts)
	require.Equal(t, "custom-openai-bandel", attempts[0].Target.Provider)
	require.Equal(t, "glm-5.2", attempts[0].Target.Model)
}

func TestPlanWith_BuiltInProviderEnforcesHardCapabilityGuard(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t, testAccountWithProvider("acc-2", "openai", 10))

	// gpt-image-1 resolves to a profile with NoTools. A tool-calling
	// requirement is non-strippable (hard), so the guard must reject it.
	required := core.NewCapabilitySet(core.CapToolCalling)
	targets := []Target{{Provider: "openai", Model: "gpt-image-1"}}

	attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, required, PlanOptions{})
	require.Error(t, err)
	require.Empty(t, attempts)
	require.Contains(t, err.Error(), "lacks required capabilities")
}

func TestPlanWith_BuiltInProviderVisionStrippableSkipsGuard(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t, testAccountWithProvider("acc-3", "openai", 10))

	// glm-5.2 resolves to a profile without vision. Vision is strippable
	// (soft-degraded by the pipeline), so the dispatch guard must NOT reject
	// it — the pipeline will strip images instead.
	required := core.NewCapabilitySet(core.CapVision)
	targets := []Target{{Provider: "openai", Model: "glm-5.2"}}

	attempts, err := d.PlanWith(ctx, schema.DefaultTenantID, targets, required, PlanOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, attempts)
}

func testAccountWithProvider(id, provider string, priority int) schema.Account {
	now := time.Now()
	return schema.Account{
		ID:        id,
		TenantID:  schema.DefaultTenantID,
		Provider:  provider,
		Label:     id,
		AuthKind:  string(schema.AuthAPIKey),
		Priority:  priority,
		Metadata:  "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestAdvanceRotationStateHonorsStickyLimit(t *testing.T) {
	cursor, nextCursor, hits := advanceRotationState(3, 0, 0, 2)
	require.Equal(t, 0, cursor)
	require.Equal(t, 0, nextCursor)
	require.Equal(t, 1, hits)

	cursor, nextCursor, hits = advanceRotationState(3, nextCursor, hits, 2)
	require.Equal(t, 0, cursor)
	require.Equal(t, 1, nextCursor)
	require.Equal(t, 0, hits)
}
