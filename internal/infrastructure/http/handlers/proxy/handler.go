package proxy

import (
	"os"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/cli/clitools"
	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/budget"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/healthcheck"
	base "github.com/bobbyunknown/flamegate/internal/infrastructure/http/handlers"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/identity"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/pipeline"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/transform"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel/cloudflare"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel/tailscale"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/update"
	"github.com/bobbyunknown/flamegate/internal/shared/consolelog"
	"github.com/bobbyunknown/flamegate/internal/shared/observ"
	"github.com/bobbyunknown/flamegate/internal/shared/usagehub"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

type Deps = base.Deps

type Handler struct {
	cfg                 config.Config
	log                 *logrus.Logger
	db                  *persistence.DB
	identity            *identity.Service
	auth                *auth.Service
	pipeline            *pipeline.Pipeline
	conns               *connectors.Registry
	chains              *persistence.ChainRepo
	aliases             *persistence.AliasRepo
	accounts            *persistence.AccountRepo
	pools               *persistence.ProxyPoolRepo
	budgets             *persistence.BudgetRepo
	budgetEngine        *budget.Engine
	usage               *persistence.UsageRepo
	resources           *persistence.ResourceRepo
	settings            *persistence.SettingsRepo
	vault               *vault.Vault
	codecs              *transform.Registry
	metrics             *observ.Metrics
	consoleLog          *consolelog.Buffer
	cliTools            *clitools.Registry
	cliToolHome         string
	dataDir             string
	cfManager           *cloudflare.Manager
	tsManager           *tailscale.Manager
	usageHub            *usagehub.Hub
	timeoutNotifier     *base.TimeoutNotifier
	proxyNotifier       *base.ProxyNotifier
	rateLimiter         interface{ SetEnabled(bool) }
	version             string
	updates             *update.Checker
	guardrails          *guardrails.Engine
	guardrailRepo       *persistence.GuardrailRepo
	guardrailLogs       *persistence.GuardrailLogRepo
	guardrailHub        *guardrails.LogHub
	guardrailTenantFlag *guardrails.SettingsTenantPolicy
	health              *persistence.HealthRepo
	healthChecker       *healthcheck.Checker
	catalog             *catalog.Service
}

func New(d base.Deps) *Handler {
	log := d.Logger
	if log == nil {
		log = logrus.StandardLogger()
	}
	cliTools := d.CLITools
	if cliTools == nil {
		cliTools = clitools.NewRegistry()
	}
	cliToolHome := d.CLITHome
	if cliToolHome == "" {
		cliToolHome, _ = os.UserHomeDir()
	}
	conLog := d.ConsoleLog
	if conLog == nil {
		conLog = consolelog.New()
	}
	return &Handler{
		cfg:                 d.Config,
		log:                 log,
		db:                  d.DB,
		identity:            d.Identity,
		auth:                d.Auth,
		pipeline:            d.Pipeline,
		conns:               d.Conns,
		chains:              d.Chains,
		aliases:             d.Aliases,
		accounts:            d.Accounts,
		pools:               d.Pools,
		budgets:             d.Budgets,
		budgetEngine:        d.BudgetEngine,
		usage:               d.Usage,
		resources:           d.Resources,
		settings:            d.Settings,
		vault:               d.Vault,
		codecs:              d.Codecs,
		metrics:             d.Metrics,
		consoleLog:          conLog,
		cliTools:            cliTools,
		cliToolHome:         cliToolHome,
		dataDir:             d.DataDir,
		cfManager:           d.CfManager,
		tsManager:           d.TsManager,
		usageHub:            d.UsageHub,
		timeoutNotifier:     d.TimeoutNotifier,
		proxyNotifier:       d.ProxyNotifier,
		rateLimiter:         d.RateLimiter,
		version:             d.Version,
		updates:             d.Updates,
		guardrails:          d.Guardrails,
		guardrailRepo:       d.GuardrailRepo,
		guardrailLogs:       d.GuardrailLogs,
		guardrailHub:        d.GuardrailHub,
		guardrailTenantFlag: d.GuardrailTenantFlags,
		health:              d.Health,
		healthChecker:       d.HealthChecker,
		catalog:             d.Catalog,
	}
}

func (s *Handler) Config() config.Config          { return s.cfg }
func (s *Handler) Logger() *logrus.Logger         { return s.log }
func (s *Handler) Identity() *identity.Service    { return s.identity }
func (s *Handler) Auth() *auth.Service            { return s.auth }
func (s *Handler) Metrics() *observ.Metrics       { return s.metrics }
func (s *Handler) ConsoleLog() *consolelog.Buffer { return s.consoleLog }
func (s *Handler) VersionString() string          { return s.versionString() }

func (s *Handler) versionString() string {
	if s.version == "" {
		return "dev"
	}
	return s.version
}
