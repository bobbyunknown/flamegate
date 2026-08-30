// Package handlers contains shared HTTP wiring types used by the admin and proxy surfaces.
package handlers

import (
	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/cli/clitools"
	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/budget"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/extstore"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/healthcheck"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/identity"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/pipeline"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/transform"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel/cloudflare"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel/tailscale"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/update"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
	"github.com/bobbyunknown/flamegate/internal/shared/consolelog"
	"github.com/bobbyunknown/flamegate/internal/shared/observ"
	"github.com/bobbyunknown/flamegate/internal/shared/usagehub"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// Deps is the HTTP adapter dependency container populated by app.Build.
type Deps struct {
	Config  config.Config
	Logger  *logrus.Logger
	Version string
	DB      *persistence.DB

	// Services
	Identity     *identity.Service
	Auth         *auth.Service
	Pipeline     *pipeline.Pipeline
	Conns        *connectors.Registry
	Codecs       *transform.Registry
	Vault        *vault.Vault
	BudgetEngine *budget.Engine
	Updates      *update.Checker
	Catalog      *catalog.Service

	// WASM extension engine (nil if no extensions).
	WASMEngine  *wasm.Engine
	WASMModules map[string]*wasm.Module

	// Repositories
	Chains        *persistence.ChainRepo
	Aliases       *persistence.AliasRepo
	Accounts      *persistence.AccountRepo
	Pools         *persistence.ProxyPoolRepo
	Budgets       *persistence.BudgetRepo
	Usage         *persistence.UsageRepo
	Resources     *persistence.ResourceRepo
	Settings      *persistence.SettingsRepo
	Health        *persistence.HealthRepo
	GuardrailRepo *persistence.GuardrailRepo
	GuardrailLogs *persistence.GuardrailLogRepo

	// Infrastructure
	ConsoleLog      *consolelog.Buffer
	CLITools        *clitools.Registry
	CLITHome        string
	DataDir         string
	CfManager       *cloudflare.Manager
	TsManager       *tailscale.Manager
	UsageHub        *usagehub.Hub
	TimeoutNotifier *TimeoutNotifier
	ProxyNotifier   *ProxyNotifier
	RateLimiter     interface{ SetEnabled(bool) }
	Metrics         *observ.Metrics

	// Guardrails
	Guardrails           *guardrails.Engine
	GuardrailHub         *guardrails.LogHub
	GuardrailTenantFlags *guardrails.SettingsTenantPolicy

	// Health
	HealthChecker *healthcheck.Checker

	// ExtStore runs the unified remote-extension install pipeline
	// (store/github/url). Populated by app.Build.
	ExtStore *extstore.Installer
}
