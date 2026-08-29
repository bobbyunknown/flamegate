package persistence

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// DB wraps GORM and exposes typed repos.
type DB struct {
	gorm    *gorm.DB
	dsn     string
	dialect string

	apiKeys         *APIKeyRepo
	accounts        *AccountRepo
	plans           *PlanRepo
	budgets         *BudgetRepo
	chains          *ChainRepo
	usage           *UsageRepo
	settings        *SettingsRepo
	tenants         *TenantRepo
	aliases         *AliasRepo
	guardrails      *GuardrailRepo
	guardrailLogs   *GuardrailLogRepo
	health          *HealthRepo
	routing         *RoutingRepo
	pools           *ProxyPoolRepo
	customProviders *CustomProviderRepo
	resources       *ResourceRepo
	users           *UserRepo
	audit           *AuditRepo
	extensions      *ExtensionRepo
	extensionModels *ExtensionModelRepo
}

func OpenDB(driver, dsn string) (*DB, error) {
	gormDB, err := Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("persistence: open: %w", err)
	}
	// SQLite :memory: is per-connection; limit pool to 1 so all queries
	// see the same database (including AutoMigrate-created tables).
	if driver == "sqlite" && (dsn == ":memory:" || dsn == "file::memory:") {
		if sqlDB, err := gormDB.DB(); err == nil {
			sqlDB.SetMaxOpenConns(1)
		}
	}
	return &DB{
		gorm:            gormDB,
		apiKeys:         NewAPIKeyRepo(gormDB),
		accounts:        NewAccountRepo(gormDB),
		plans:           NewPlanRepo(gormDB),
		budgets:         NewBudgetRepo(gormDB),
		chains:          NewChainRepo(gormDB),
		usage:           NewUsageRepo(gormDB),
		settings:        NewSettingsRepo(gormDB),
		tenants:         NewTenantRepo(gormDB),
		aliases:         NewAliasRepo(gormDB),
		guardrails:      NewGuardrailRepo(gormDB),
		guardrailLogs:   NewGuardrailLogRepo(gormDB),
		health:          NewHealthRepo(gormDB),
		routing:         NewRoutingRepo(gormDB),
		pools:           NewProxyPoolRepo(gormDB),
		customProviders: NewCustomProviderRepo(gormDB),
		resources:       NewResourceRepo(gormDB),
		users:           NewUserRepo(gormDB),
		audit:           NewAuditRepo(gormDB),
		extensions:      NewExtensionRepo(gormDB),
		extensionModels: NewExtensionModelRepo(gormDB),
	}, nil
}

// SQL returns the raw *sql.DB for advanced queries (PRAGMA, etc).
func (db *DB) SQL() *sql.DB {
	sqlDB, _ := db.gorm.DB()
	return sqlDB
}

// Dialect returns the active SQL dialect.
func (db *DB) Dialect() schema.Dialect {
	return schema.Dialect(db.dialect)
}

// SQLitePath returns the database file path (only meaningful for SQLite).
func (db *DB) SQLitePath() string {
	return db.dsn
}

// Gorm returns the underlying *gorm.DB for advanced queries.
func (db *DB) Gorm() *gorm.DB { return db.gorm }

// Accessor methods — mirror store.DB pattern
func (db *DB) APIKeys() *APIKeyRepo                 { return db.apiKeys }
func (db *DB) Accounts() *AccountRepo               { return db.accounts }
func (db *DB) Plans() *PlanRepo                     { return db.plans }
func (db *DB) Budgets() *BudgetRepo                 { return db.budgets }
func (db *DB) Chains() *ChainRepo                   { return db.chains }
func (db *DB) Usage() *UsageRepo                    { return db.usage }
func (db *DB) Settings() *SettingsRepo              { return db.settings }
func (db *DB) Tenants() *TenantRepo                 { return db.tenants }
func (db *DB) Aliases() *AliasRepo                  { return db.aliases }
func (db *DB) Guardrails() *GuardrailRepo           { return db.guardrails }
func (db *DB) GuardrailLogs() *GuardrailLogRepo     { return db.guardrailLogs }
func (db *DB) Health() *HealthRepo                  { return db.health }
func (db *DB) Routing() *RoutingRepo                { return db.routing }
func (db *DB) ProxyPools() *ProxyPoolRepo           { return db.pools }
func (db *DB) CustomProviders() *CustomProviderRepo { return db.customProviders }
func (db *DB) Resources() *ResourceRepo             { return db.resources }
func (db *DB) Audit() *AuditRepo                    { return db.audit }
func (db *DB) Users() *UserRepo                     { return db.users }
func (db *DB) Extensions() *ExtensionRepo           { return db.extensions }
func (db *DB) ExtensionModels() *ExtensionModelRepo { return db.extensionModels }

// Migrate runs GORM AutoMigrate for all schema models.
func (db *DB) Migrate() error {
	return db.gorm.AutoMigrate(schema.AllModels()...)
}

// EnsureDefault seeds the default tenant and default plan templates.
func (db *DB) EnsureDefault() error {
	if err := db.tenants.EnsureDefault(context.TODO()); err != nil {
		return err
	}
	return db.plans.EnsureDefault(context.TODO(), schema.DefaultTenantID)
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	sqlDB, err := db.gorm.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// BeginTx starts a GORM transaction. Callers use Commit()/Rollback() on the returned *gorm.DB.
func (db *DB) BeginTx(ctx context.Context) *gorm.DB {
	return db.gorm.WithContext(ctx).Begin()
}

// SetPool configures database connection pool parameters.
func (db *DB) SetPool(maxOpen, maxIdle int) {
	if sqlDB, err := db.gorm.DB(); err == nil {
		if maxOpen > 0 {
			sqlDB.SetMaxOpenConns(maxOpen)
		}
		if maxIdle > 0 {
			sqlDB.SetMaxIdleConns(maxIdle)
		}
	}
}

