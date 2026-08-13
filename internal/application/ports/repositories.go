// Package ports defines application-layer interfaces (driven ports) that
// infrastructure adapters must implement. Only domain and stdlib imports are
// allowed; no GORM, HTTP, or connector imports.
//
// TODO: entity types currently reference store.Models — these will move to
// domain/ in Phase 5 (GORM migration). After that, ports import only domain.
package ports

import (
	"context"
	"database/sql"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// APIKeyRepository reads and writes API key aggregates.
type APIKeyRepository interface {
	Create(ctx context.Context, k schema.APIKey) error
	CreateOnTx(ctx context.Context, tx *sql.Tx, k schema.APIKey) error
	Get(ctx context.Context, id string) (schema.APIKey, error)
	FindByLookup(ctx context.Context, lookup string) (schema.APIKey, error)
	List(ctx context.Context, tenantID string) ([]schema.APIKey, error)
	TouchLastUsed(ctx context.Context, id string, at time.Time) error
	SetDisabled(ctx context.Context, id string, disabled bool) error
	Delete(ctx context.Context, id string) error
	SetPlanID(ctx context.Context, id string, planID string) error
	SetAllowedModels(ctx context.Context, keyID string, models []string) error
	GetAllowedModels(ctx context.Context, keyID string) ([]string, error)
	IsModelAllowed(ctx context.Context, keyID string, model string) (bool, error)
	SetAllowedModelsOnTx(ctx context.Context, tx *sql.Tx, keyID string, models []string) error
}

// ---------------------------------------------------------------------------
// Account
// ---------------------------------------------------------------------------

// AccountRepository reads and writes provider account aggregates.
type AccountRepository interface {
	Create(ctx context.Context, a schema.Account) error
	Get(ctx context.Context, id string) (schema.Account, error)
	ListByProvider(ctx context.Context, tenantID, provider string) ([]schema.Account, error)
	ListByTenant(ctx context.Context, tenantID string) ([]schema.Account, error)
	Update(ctx context.Context, a schema.Account) error
	Delete(ctx context.Context, id string) error
	SetCooldown(ctx context.Context, id string, until time.Time) error
	SetBackoffLevel(ctx context.Context, id string, level int) error
	ResetBackoffLevel(ctx context.Context, id string) error
	ClearExpiredCooldowns(ctx context.Context) (int64, error)
	ClearProviderCooldowns(ctx context.Context, tenantID, provider string) error
	UpdateTokens(ctx context.Context, a schema.Account) error
	SetNeedsReconnect(ctx context.Context, id string, flag bool) error
}

// ---------------------------------------------------------------------------
// Plan
// ---------------------------------------------------------------------------

// PlanRepository reads and writes plan templates.
type PlanRepository interface {
	Create(ctx context.Context, p schema.Plan) error
	CreateOnTx(ctx context.Context, tx *sql.Tx, p schema.Plan) error
	Get(ctx context.Context, id string) (schema.Plan, error)
	List(ctx context.Context, tenantID string) ([]schema.Plan, error)
	Update(ctx context.Context, p schema.Plan) error
	Delete(ctx context.Context, id string) error
	CountKeys(ctx context.Context, planID string) (int, error)
}

// ---------------------------------------------------------------------------
// Budget
// ---------------------------------------------------------------------------

// BudgetRepository reads and write spend limits.
type BudgetRepository interface {
	Create(ctx context.Context, b schema.Budget) error
	CreateOnTx(ctx context.Context, tx *sql.Tx, b schema.Budget) error
	Get(ctx context.Context, id string) (schema.Budget, error)
	ListByScope(ctx context.Context, kind schema.BudgetScope, scopeID string) ([]schema.Budget, error)
	ListByTenant(ctx context.Context, tenantID string) ([]schema.Budget, error)
	Update(ctx context.Context, b schema.Budget) error
	Delete(ctx context.Context, id string) error
}

// ---------------------------------------------------------------------------
// Chain
// ---------------------------------------------------------------------------

// ChainRepository reads and writes routing chains.
type ChainRepository interface {
	Create(ctx context.Context, c schema.Chain) error
	Get(ctx context.Context, id string) (schema.Chain, error)
	ListByTenant(ctx context.Context, tenantID string) ([]schema.Chain, error)
	Update(ctx context.Context, c schema.Chain) error
	Delete(ctx context.Context, id string) error
}

// ---------------------------------------------------------------------------
// Usage
// ---------------------------------------------------------------------------

// UsageRepository records and aggregates metering data.
type UsageRepository interface {
	Record(ctx context.Context, u schema.UsageRecord) error
	RecordBatch(ctx context.Context, records []schema.UsageRecord) error
	SpendSince(ctx context.Context, scope schema.BudgetScope, scopeID string, since time.Time) (int64, error)
	SpendAndTokens(ctx context.Context, scope schema.BudgetScope, scopeID string, since time.Time) (costMicros int64, tokens int64, err error)
	SpendAndTokensBatch(ctx context.Context, scopes []schema.SpendScope) ([]schema.SpendResult, error)
	Summarize(ctx context.Context, tenantID string, since time.Time) (schema.Summary, error)
	SummarizeByKey(ctx context.Context, keyID string, since time.Time) (schema.Summary, error)
	Breakdown(ctx context.Context, tenantID string, since time.Time) ([]schema.ProviderUsage, error)
	Recent(ctx context.Context, tenantID string, limit int) ([]schema.RecentRecord, error)
	ByAccount(ctx context.Context, tenantID string, since time.Time) ([]schema.AccountUsage, error)
	ByModel(ctx context.Context, tenantID string, since time.Time) ([]schema.ModelUsage, error)
	ByModelByKey(ctx context.Context, keyID string, since time.Time) ([]schema.ModelUsage, error)
	Timeline(ctx context.Context, tenantID string, since time.Time, to time.Time, buckets int) ([]schema.TimeBucket, error)
	DailyByKey(ctx context.Context, keyID string, since time.Time) ([]schema.DailyPoint, error)
	SavingsByRule(ctx context.Context, tenantID string, since time.Time) ([]schema.RuleSavings, error)
	SavingsByClient(ctx context.Context, tenantID string, since time.Time) ([]schema.ClientSavings, error)
}

// ---------------------------------------------------------------------------
// Guardrail
// ---------------------------------------------------------------------------

// GuardrailRepository reads and writes guardrail policies.
type GuardrailRepository interface {
	Upsert(ctx context.Context, p schema.GuardrailPolicy) error
	Get(ctx context.Context, id string) (schema.GuardrailPolicy, error)
	GetByScope(ctx context.Context, tenantID string, scope schema.GuardrailScope, scopeID string) (schema.GuardrailPolicy, error)
	List(ctx context.Context, tenantID string, scope schema.GuardrailScope) ([]schema.GuardrailPolicy, error)
	Delete(ctx context.Context, id string) error
}

// GuardrailLogRepository persists detector audit logs.
type GuardrailLogRepository interface {
	Insert(ctx context.Context, e schema.GuardrailLog) error
	BatchInsert(ctx context.Context, entries []schema.GuardrailLog) error
	DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	List(ctx context.Context, tenantID string, f schema.GuardrailLogFilter) ([]schema.GuardrailLog, error)
}

// ---------------------------------------------------------------------------
// Settings / Tenant / Audit
// ---------------------------------------------------------------------------

// SettingsRepository is a key/value configuration store.
type SettingsRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// TenantRepository reads and writes tenants.
type TenantRepository interface {
	EnsureDefault(ctx context.Context) error
	Upsert(ctx context.Context, t schema.Tenant) error
	Get(ctx context.Context, id string) (schema.Tenant, error)
}

// AuditRepository appends and reads audit entries.
type AuditRepository interface {
	Append(ctx context.Context, e schema.AuditEntry) error
	List(ctx context.Context, tenantID string, limit int) ([]schema.AuditEntry, error)
}

// ---------------------------------------------------------------------------
// Alias
// ---------------------------------------------------------------------------

// AliasRepository reads and writes model aliases.
type AliasRepository interface {
	List(ctx context.Context) ([]schema.ModelAlias, error)
	Get(ctx context.Context, alias string) (schema.ModelAlias, error)
	Set(ctx context.Context, alias, target string) error
	Delete(ctx context.Context, alias string) error
}

// ---------------------------------------------------------------------------
// Proxy Pool
// ---------------------------------------------------------------------------

// ProxyPoolRepository reads and writes proxy pools.
type ProxyPoolRepository interface {
	Get(ctx context.Context, id string) (schema.ProxyPool, error)
	List(ctx context.Context) ([]schema.ProxyPool, error)
	Create(ctx context.Context, p schema.ProxyPool) error
	Update(ctx context.Context, p schema.ProxyPool) error
	Delete(ctx context.Context, id string) error
}

// ---------------------------------------------------------------------------
// Resource
// ---------------------------------------------------------------------------

// ResourceRepository persists resource monitor samples.
type ResourceRepository interface {
	InsertResourceSample(ctx context.Context, s schema.ResourceSample) error
	ResourceBuckets(ctx context.Context, since time.Time, interval time.Duration) ([]schema.ResourceBucket, error)
	PruneResourceSamples(ctx context.Context, maxAge time.Duration) error
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// HealthRepository persists account/model health probe results.
type HealthRepository interface {
	Upsert(ctx context.Context, h schema.AccountHealth) error
	Get(ctx context.Context, accountID, model string) (schema.AccountHealth, error)
	IsUnhealthy(ctx context.Context, accountID, model string) (bool, error)
	MarkHealthy(ctx context.Context, accountID, model string) error
	List(ctx context.Context, tenantID string) ([]schema.AccountHealth, error)
	RecentAccountModels(ctx context.Context, tenantID string, since time.Time, limit int) ([]schema.AccountHealth, error)
}

// ---------------------------------------------------------------------------
// Routing
// ---------------------------------------------------------------------------

// RoutingRepository handles model-level cooldowns and chain rotation state.
type RoutingRepository interface {
	SetModelCooldown(ctx context.Context, accountID, model string, until time.Time) error
	ClearModelCooldown(ctx context.Context, accountID, model string) error
	ClearAccountModelCooldowns(ctx context.Context, accountID string) error
	IsModelCooldownActive(ctx context.Context, accountID, model string) (bool, error)
	ExpireModelCooldowns(ctx context.Context) (int64, error)
	GetChainRotation(ctx context.Context, chainID string) (int, error)
	SetChainRotation(ctx context.Context, chainID string, index int) error
	GetChainRotationState(ctx context.Context, chainID string) (schema.ChainRotation, error)
	SetChainRotationState(ctx context.Context, state schema.ChainRotation) error
	GetTargetRotationState(ctx context.Context, scopeKey string) (schema.TargetRotation, error)
	SetTargetRotationState(ctx context.Context, state schema.TargetRotation) error
	GetAccountAffinity(ctx context.Context, scopeKey string) (schema.AccountAffinity, error)
	SetAccountAffinity(ctx context.Context, state schema.AccountAffinity) error
	ExpireAccountAffinities(ctx context.Context) (int64, error)
}

// ---------------------------------------------------------------------------
// Custom Provider
// ---------------------------------------------------------------------------

// CustomProviderRepository reads and writes custom provider instances and models.
type CustomProviderRepository interface {
	ListProviders(ctx context.Context, tenantID string) ([]schema.CustomProvider, error)
	GetProvider(ctx context.Context, id string) (schema.CustomProvider, error)
	CreateProvider(ctx context.Context, p schema.CustomProvider) error
	UpdateProvider(ctx context.Context, p schema.CustomProvider) error
	DeleteProvider(ctx context.Context, id string) error
	ListModels(ctx context.Context, tenantID string) ([]schema.CustomModel, error)
	ListModelsByProvider(ctx context.Context, providerID string) ([]schema.CustomModel, error)
	CreateModel(ctx context.Context, m schema.CustomModel) error
	UpdateModel(ctx context.Context, m schema.CustomModel) error
	GetModel(ctx context.Context, id string) (schema.CustomModel, error)
	DeleteModel(ctx context.Context, id string) error
}

// ---------------------------------------------------------------------------
// Extension
// ---------------------------------------------------------------------------

// ExtensionRepository reads and writes WASM extension metadata.
type ExtensionRepository interface {
	Get(ctx context.Context, id string) (schema.Extension, error)
	FindBySlug(ctx context.Context, slug string) (schema.Extension, error)
	List(ctx context.Context, tenantID string) ([]schema.Extension, error)
	ListByState(ctx context.Context, state string) ([]schema.Extension, error)
	Create(ctx context.Context, e schema.Extension) error
	Update(ctx context.Context, e schema.Extension) error
	Delete(ctx context.Context, id string) error
}

// ExtensionModelRepository reads and writes models registered by extensions.
type ExtensionModelRepository interface {
	Get(ctx context.Context, id string) (schema.ExtensionModel, error)
	ListByExtension(ctx context.Context, extensionID string) ([]schema.ExtensionModel, error)
	ListBySource(ctx context.Context, extensionID, source string) ([]schema.ExtensionModel, error)
	ListByTenant(ctx context.Context, tenantID string) ([]schema.ExtensionModel, error)
	Create(ctx context.Context, m schema.ExtensionModel) error
	Update(ctx context.Context, m schema.ExtensionModel) error
	Delete(ctx context.Context, id string) error
	DeleteBySource(ctx context.Context, extensionID, source string) error
	DeleteByExtension(ctx context.Context, extensionID string) error
}
