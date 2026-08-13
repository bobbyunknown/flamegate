package ports

import "context"

// Tx exposes transaction-scoped repository handles. All repositories returned
// by Tx operate within the same database transaction. The transaction commits
// when Run returns nil and rolls back on any error.
//
// TODO: add remaining repositories as they are needed by usecases.
type Tx interface {
	APIKeys() APIKeyRepository
	Accounts() AccountRepository
	Plans() PlanRepository
	Budgets() BudgetRepository
	Chains() ChainRepository
	Usage() UsageRepository
	Guardrails() GuardrailRepository
	GuardrailLogs() GuardrailLogRepository
	Settings() SettingsRepository
	Tenants() TenantRepository
	Audit() AuditRepository
	Aliases() AliasRepository
	ProxyPools() ProxyPoolRepository
	Resources() ResourceRepository
	Health() HealthRepository
	Routing() RoutingRepository
	CustomProviders() CustomProviderRepository
}

// UnitOfWork executes a function inside a single database transaction.
// Infrastructure adapters wrap BEGIN/COMMIT/ROLLBACK around fn.
type UnitOfWork interface {
	Run(ctx context.Context, fn func(ctx context.Context, tx Tx) error) error
}
