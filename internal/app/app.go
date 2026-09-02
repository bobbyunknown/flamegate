// Package app wires FlameGate's components into a runnable application: it
// opens the store, runs migrations, loads or generates the master key, and
// constructs the gateway with its full dependency graph.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"

	"github.com/google/uuid"

	"github.com/bobbyunknown/flamegate/internal/config"
	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/budget"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/cache"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/capability"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/catalog"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/extstore"
	"github.com/bobbyunknown/flamegate/internal/shared/crypto"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/dispatch"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails/bias"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails/injection"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails/pii"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails/topics"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/guardrails/toxicity"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/healthcheck"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/handlers"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/router"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/identity"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/meter"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/pipeline"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/transform"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel/cloudflare"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/tunnel/tailscale"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/update"
	"github.com/bobbyunknown/flamegate/internal/shared/httputil"
	"github.com/bobbyunknown/flamegate/internal/shared/limits"
	"github.com/bobbyunknown/flamegate/internal/shared/observ"
	"github.com/bobbyunknown/flamegate/internal/shared/slimmer"
	"github.com/bobbyunknown/flamegate/internal/shared/usagehub"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// App is the assembled application, ready to serve.
type App struct {
	cfg                config.Config
	log                *logrus.Logger
	db                 *persistence.DB
	accounts           *persistence.AccountRepo
	server             *http.Server
	proxyServer        *http.Server // optional dedicated /v1 listener (server.proxy_port)
	guardrailAudit     *guardrails.AuditWriter
	guardrailRetention *guardrails.RetentionSweeper
	meter              *meter.Meter
	healthChecker      *healthcheck.Checker
	wasmEngine         *wasm.Engine
	wasmModules        map[string]*wasm.Module
	catalogSvc         *catalog.Service

	// bg tracks long-lived background workers that touch the DB (health
	// checker, cooldown sweeper) so shutdown can wait for
	// them to return before closing the store, avoiding a use-after-close race.
	bg sync.WaitGroup
}

// Build constructs the application from configuration. It opens the database,
// applies migrations, initializes the crypto root, and wires the gateway.
func Build(ctx context.Context, cfg config.Config, log *logrus.Logger, version string) (*App, error) {
	httputil.SetAllowPrivateBaseURL(cfg.Security.AllowPrivateBaseURL)

	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		return nil, err
	}

	driver := cfg.Database.Driver
	if driver == "" {
		driver = "sqlite"
	}
	dsn := cfg.Database.DSN
	if driver == "sqlite" && dsn == "" {
		dsn = filepath.Join(dataDir, "flamegate.db")
	}
	db, err := persistence.OpenDB(driver, dsn)
	if err != nil {
		return nil, err
	}
	db.SetPool(cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns)
	if err := db.Migrate(); err != nil {
		return nil, fmt.Errorf("app: migrate: %w", err)
	}
	if err := db.EnsureDefault(); err != nil {
		return nil, fmt.Errorf("app: ensure default tenant: %w", err)
	}

	// Clear stale cooldowns left over from a previous session so accounts
	// are immediately usable after a restart.
	if cleared, cerr := db.Accounts().ClearExpiredCooldowns(ctx); cerr != nil {
		log.Warn("failed to clear expired cooldowns", "err", cerr)
	} else if cleared > 0 {
		log.Info("cleared stale account cooldowns from previous session", "count", cleared)
	}

	masterKey, err := LoadOrCreateMasterKey(cfg, dataDir, log)
	if err != nil {
		return nil, err
	}
	sealer, err := crypto.NewSealer(masterKey)
	if err != nil {
		return nil, fmt.Errorf("app: build sealer: %w", err)
	}

	// Construct services.
	v := vault.New(sealer)
	connRegistry := connectors.DefaultRegistry()

	// WASM extension engine. Compiles installed extensions and provides
	// connector fallback for non-native providers.
	wasmEngine := wasm.NewEngine(cfg.WASM, v, db.Accounts(), httputil.NewClient(60*time.Second))
	wasmModules := map[string]*wasm.Module{}
	if exts, scanErr := wasm.Scan(cfg.WASM.ExtDir); scanErr == nil {
		for _, ext := range exts {
			if compileErr := wasmEngine.Compile(ctx, ext.Slug, ext.WasmBytes, wasm.ExtensionConfig{
				Slug:         ext.Slug,
				Timeout:      time.Duration(ext.Schema.Timeout) * time.Second,
				AllowedHosts: cfg.WASM.AllowedHosts,
				Entrypoints:  ext.Schema.Entrypoints,
			}); compileErr != nil {
				log.WithFields(logrus.Fields{"slug": ext.Slug, "err": compileErr}).Warn("WASM extension compile failed")
				continue
			}
			log.WithFields(logrus.Fields{"slug": ext.Slug, "version": ext.Schema.Version}).Info("WASM extension loaded")
			wasmModules[ext.Slug] = wasm.NewModule(wasmEngine, ext.Slug, cfg.WASM.MaxInst)
			display := ext.Schema.Name
			if display == "" {
				display = ext.Slug
			}
			connectors.RegisterExtensionSpec(connectors.ProviderSpec{
				ID:           ext.Slug,
				DisplayName:  display,
				Alias:        ext.Schema.Alias,
				Aliases:      ext.Schema.Aliases,
				Dialect:      core.DialectOpenAI,
				AuthKind:     firstAuthModeOf(ext.Schema.AuthModes),
				AuthModes:    ext.Schema.AuthModes,
				ServiceKinds: []core.ServiceKind{core.ServiceLLM},
				Notice:       "WASM extension",
			})
		}
	}
	if wasmEngine.HasExtensions() {
		connRegistry.SetWASMFallback(func(slug string) (core.Connector, bool) {
			return wasmEngine.GetConnector(slug)
		})
		connRegistry.SetForceWasmAll(cfg.WASM.ForceWasmAll)
		log.WithFields(logrus.Fields{"extensions": len(wasmEngine.Slugs()), "force_wasm_all": cfg.WASM.ForceWasmAll}).Info("WASM fallback registered")
	}
	// Remote extension installer shared by the admin API and CLI. It reuses the
	// same WASM engine, vault, and ExtensionRepo as startup scanning.
	extStore := extstore.NewInstallerFromConfig(cfg.WASM, httputil.NewClient(60*time.Second), wasmEngine, db.Extensions())
	// Load user-defined dynamic custom providers and their custom models from
	// the database so they are routable and discoverable immediately at startup.
	loadCustomProviders(ctx, db, log)
	codecs := transform.DefaultRegistry()

	idSvc := identity.New(db.APIKeys())

	authSvc := auth.New(db.Users(), db.Settings(), cfg.Security.JWTSecret, cfg.Security.SessionTTL)
	seeded, err := authSvc.EnsureDefaults(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: init auth: %w", err)
	}
	if seeded {
		log.Warn("seeded default dashboard password",
			"password", auth.DefaultPassword,
			"note", "change it on first login via the onboarding flow")
	}

	catalogCachePath := cfg.Catalog.CachePath
	if catalogCachePath == "" {
		catalogCachePath = filepath.Join(dataDir, "models.json")
	}
	catalogSvc := catalog.NewService(catalog.Config{
		Enabled:      cfg.Catalog.Enabled,
		URL:          cfg.Catalog.URL,
		SyncInterval: cfg.Catalog.SyncInterval,
		CachePath:    catalogCachePath,
		Timeout:      cfg.Catalog.Timeout,
	})
	if err := catalogSvc.LoadCache(); err != nil {
		log.WithError(err).Warn("failed to load local catalog cache")
	}
	capability.SetCatalogSource(catalogSvc)

	pricing := buildPricing()
	modelPrices := buildModelPrices()
	mtr := meter.New(db.Usage(), pricing, modelPrices)
	mtr.SetPriceSource(catalogSvc)
	mtr.EnableAsync(meter.AsyncConfig{
		Enabled:              cfg.Meter.Async,
		BatchSize:            cfg.Meter.BatchSize,
		FlushInterval:        cfg.Meter.FlushInterval,
		QueueSize:            cfg.Meter.QueueSize,
		FullQueuePolicy:      cfg.Meter.FullQueuePolicy,
		ShutdownFlushTimeout: cfg.Meter.ShutdownFlushTimeout,
		Logger:               log,
	})
	uh := usagehub.New()
	mtr.SetHub(uh)
	bud := budget.New(db.Budgets(), db.Usage())
	refresher := auth.NewTokenRefresher(db.Accounts(), v, wasmEngine, log.WithField("subsystem", "refresher"))
	disp := dispatch.New(connRegistry, db.Accounts(), v)
	disp.SetRefresher(refresher)
	// Resolve proxy pool bindings for accounts that have one.
	disp.SetPoolSource(db.ProxyPools())
	// Model-level cooldowns, round-robin chain rotation, and background health.
	disp.SetRoutingSource(db.Routing())
	disp.SetHealthSource(db.Health())
	slim := slimmer.Default()
	metrics := observ.New()

	// Semantic cache. Defaults to a local in-memory store keyed by a
	// deterministic hash embedder (exact-prompt caching). When configured with
	// backend=redis + embedding_provider=api, uses Redis for persistence and
	// an embedding API for true semantic near-match caching.
	var cacheStore cache.Store
	if cfg.Cache.Backend == "redis" && cfg.Cache.RedisURL != "" {
		rs, err := cache.NewRedisStore(cache.RedisStoreConfig{
			URL:    cfg.Cache.RedisURL,
			TTL:    cfg.Cache.TTL,
			Logger: log,
		})
		if err != nil {
			return nil, fmt.Errorf("app: redis cache store: %w", err)
		}
		cacheStore = rs
		log.Info("semantic cache: redis backend", "url", cfg.Cache.RedisURL)
	}

	var embedder cache.Embedder
	if cfg.Cache.EmbeddingProvider == "api" && cfg.Cache.EmbeddingAPIKey != "" {
		embedder = cache.NewAPIEmbedder(cache.APIEmbedderConfig{
			BaseURL: cfg.Cache.EmbeddingAPIURL,
			APIKey:  cfg.Cache.EmbeddingAPIKey,
			Model:   cfg.Cache.EmbeddingModel,
			Dims:    cfg.Cache.EmbeddingDims,
		})
		log.Info("semantic cache: API embedder", "model", cfg.Cache.EmbeddingModel)
	} else {
		embedder = cache.NewHashEmbedder(32)
		if cfg.Cache.Enabled {
			log.Info("semantic cache: hash embedder (exact-match only)")
		}
	}

	semanticCache := cache.New(cache.Config{
		Enabled:             cfg.Cache.Enabled,
		SimilarityThreshold: cfg.Cache.SimilarityThreshold,
		TTL:                 cfg.Cache.TTL,
		MaxEntries:          10000,
	}, cacheStore)

	// Timeout notifier: atomic cache of timeout values that can be updated
	// at runtime from the dashboard settings without restarting.
	timeoutNotifier := handlers.NewTimeoutNotifier(
		cfg.Server.StreamStallTimeout,
		30*time.Second, // ResponseHeaderTimeout fallback (from transport)
		cfg.Server.RequestTimeout,
	)

	// Proxy notifier: atomic cache of outbound proxy config that can be updated
	// at runtime from the dashboard settings without restarting.
	var proxyEnabled bool
	var proxyURL, noProxy string
	if raw, err := db.Settings().Get(ctx, "endpoint_settings"); err == nil && raw != "" {
		var es struct {
			OutboundProxyEnabled bool   `json:"outbound_proxy_enabled"`
			OutboundProxyURL     string `json:"outbound_proxy_url"`
			OutboundNoProxy      string `json:"outbound_no_proxy"`
		}
		if json.Unmarshal([]byte(raw), &es) == nil {
			proxyEnabled = es.OutboundProxyEnabled
			proxyURL = es.OutboundProxyURL
			noProxy = es.OutboundNoProxy
		}
	}
	proxyNotifier := handlers.NewProxyNotifier(proxyEnabled, proxyURL, noProxy)
	disp.SetGlobalProxy(proxyNotifier)

	cfg.Limits.Enabled = true
	if raw, err := db.Settings().Get(ctx, "endpoint_settings"); err == nil && raw != "" {
		var es struct {
			RateLimitsEnabled *bool `json:"rate_limits_enabled"`
		}
		if json.Unmarshal([]byte(raw), &es) == nil && es.RateLimitsEnabled != nil {
			cfg.Limits.Enabled = *es.RateLimitsEnabled
		}
	}

	// Guardrails: content-safety policies layered global → provider → model →
	// chain → apikey. Resolver caches lookups for 30s; audit writer drains a
	// buffered channel to the guardrail_logs table.
	guardrailResolver := guardrails.NewResolver(db.Guardrails(), 30*time.Second)
	// Audit log hub: AuditWriter publishes successfully-flushed rows here so
	// the dashboard's Logs tab can subscribe via SSE for near-real-time
	// updates without polling.
	guardrailLogHub := guardrails.NewLogHub()
	guardrailAudit := guardrails.NewAuditWriter(db.GuardrailLogs(), log, guardrails.AuditWriterConfig{Hub: guardrailLogHub})
	// Toxicity: native engine ships unconditionally; OpenAI Moderation is
	// wired only when an API key is configured.
	toxCfg := toxicity.Config{}
	if cfg.Guardrails.Toxicity.OpenAIAPIKey != "" {
		toxCfg.OpenAI = &toxicity.OpenAIConfig{
			APIKey:  cfg.Guardrails.Toxicity.OpenAIAPIKey,
			BaseURL: cfg.Guardrails.Toxicity.OpenAIBaseURL,
			Model:   cfg.Guardrails.Toxicity.OpenAIModel,
			Timeout: cfg.Guardrails.Toxicity.OpenAITimeout,
		}
		log.Info("guardrails: toxicity OpenAI engine enabled")
	}
	// PII: native recognizers always ship; Presidio sidecar is wired only
	// when an analyzer URL is configured. Policies opt in per-tenant via
	// PIIConfig.Engine = "presidio".
	piiCfg := pii.Config{}
	if cfg.Guardrails.PII.PresidioAnalyzerURL != "" {
		piiCfg.Presidio = pii.NewPresidioEngine(pii.PresidioConfig{
			AnalyzerURL: cfg.Guardrails.PII.PresidioAnalyzerURL,
			Timeout:     cfg.Guardrails.PII.PresidioTimeout,
			Language:    cfg.Guardrails.PII.PresidioLanguage,
		})
		log.Info("guardrails: PII Presidio engine enabled", "analyzer", cfg.Guardrails.PII.PresidioAnalyzerURL)
	}
	// Per-tenant guardrails settings (currently just allow_external_engines).
	guardrailTenantPolicy := guardrails.NewSettingsTenantPolicy(db.Settings(), 30*time.Second)
	guardrailEngine := guardrails.NewEngine(guardrails.EngineConfig{
		Resolver: guardrailResolver,
		Audit:    guardrailAudit,
		Detectors: []guardrails.Detector{
			pii.NewWithConfig(piiCfg),
			injection.New(),
			topics.New(topics.Config{Embedder: embedder}),
			toxicity.New(toxCfg),
			bias.New(),
		},
		Logger:       log,
		Metrics:      metrics,
		TenantPolicy: guardrailTenantPolicy,
	})
	// Retention sweeper: deletes guardrail_logs older than N days.
	var guardrailRetention *guardrails.RetentionSweeper
	if cfg.Guardrails.AuditRetentionDays > 0 {
		guardrailRetention = guardrails.NewRetentionSweeper(db.GuardrailLogs(), log, guardrails.RetentionConfig{
			Retention: time.Duration(cfg.Guardrails.AuditRetentionDays) * 24 * time.Hour,
		})
		guardrailRetention.Start()
		log.Info("guardrails: audit retention sweeper started", "days", cfg.Guardrails.AuditRetentionDays)
	}

	limiter := limits.NewMemory(limits.MemoryConfig{
		Enabled:         cfg.Limits.Enabled,
		Window:          cfg.Limits.Window,
		CleanupInterval: cfg.Limits.CleanupInterval,
	})

	pipe := pipeline.New(pipeline.Deps{
		Dispatcher:         disp,
		Refresher:          refresher,
		Meter:              mtr,
		Budget:             bud,
		Slimmer:            slim,
		Metrics:            metrics,
		Cache:              semanticCache,
		Embedder:           embedder,
		Guardrails:         guardrailEngine,
		Limiter:            limiter,
		Logger:             log,
		RequestTimeout:     cfg.Server.RequestTimeout,
		StreamStallTimeout: cfg.Server.StreamStallTimeout,
		TimeoutReader:      timeoutNotifier,
	})

	// Tunnel managers for Cloudflare quick tunnel and Tailscale funnel.
	cfManager := cloudflare.NewManager(dataDir, cfg.Server.Port, log)
	tsManager := tailscale.NewManager(dataDir, cfg.Server.Port, log)

	healthChecker := healthcheck.New(healthcheck.Config{
		Enabled:              cfg.Health.Enabled,
		Interval:             cfg.Health.Interval,
		Timeout:              cfg.Health.Timeout,
		MaxParallel:          cfg.Health.MaxParallel,
		FailureThreshold:     cfg.Health.FailureThreshold,
		SuccessThreshold:     cfg.Health.SuccessThreshold,
		RecentModelWindow:    cfg.Health.RecentModelWindow,
		MaxModelsPerProvider: cfg.Health.MaxModelsPerProvider,
	}, log, db.Accounts(), db.Health(), connRegistry, v)

	gw := router.New(handlers.Deps{
		Config:               cfg,
		Logger:               log,
		Version:              version,
		DB:                   db,
		Updates:              update.NewChecker(version, ""),
		WASMEngine:           wasmEngine,
		WASMModules:          wasmModules,
		ExtStore:             extStore,
		Identity:             idSvc,
		Auth:                 authSvc,
		Pipeline:             pipe,
		Conns:                connRegistry,
		Chains:               db.Chains(),
		Aliases:              db.Aliases(),
		Accounts:             db.Accounts(),
		Pools:                db.ProxyPools(),
		Budgets:              db.Budgets(),
		BudgetEngine:         bud,
		Usage:                db.Usage(),
		Resources:            db.Resources(),
		Settings:             db.Settings(),
		Vault:                v,
		Codecs:               codecs,
		Metrics:              metrics,
		DataDir:              dataDir,
		CfManager:            cfManager,
		TsManager:            tsManager,
		UsageHub:             uh,
		TimeoutNotifier:      timeoutNotifier,
		ProxyNotifier:        proxyNotifier,
		RateLimiter:          limiter,
		Guardrails:           guardrailEngine,
		GuardrailRepo:        db.Guardrails(),
		GuardrailLogs:        db.GuardrailLogs(),
		GuardrailHub:         guardrailLogHub,
		GuardrailTenantFlags: guardrailTenantPolicy,
		Health:               db.Health(),
		HealthChecker:        healthChecker,
		Catalog:              catalogSvc,
	})

	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           gw.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		// No WriteTimeout: streaming responses are long-lived; the stall
		// timeout is enforced per-stream inside the connectors instead.
	}

	// Optional dedicated /v1 listener (server.proxy_port). Admin stays on Port.
	var proxySrv *http.Server
	if addr := cfg.ProxyAddr(); addr != "" {
		proxySrv = &http.Server{
			Addr:              addr,
			Handler:           gw.ProxyHandler(),
			ReadHeaderTimeout: 15 * time.Second,
		}
	}

	// Auto-seed accounts for free, no-auth providers so they are immediately
	// usable without a manual "connect" step in the dashboard.
	seedFreeAccounts(ctx, db.Accounts(), log)

	return &App{
		cfg:                cfg,
		log:                log,
		db:                 db,
		accounts:           db.Accounts(),
		server:             srv,
		proxyServer:        proxySrv,
		guardrailAudit:     guardrailAudit,
		guardrailRetention: guardrailRetention,
		meter:              mtr,
		healthChecker:      healthChecker,
		wasmEngine:         wasmEngine,
		wasmModules:        wasmModules,
		catalogSvc:         catalogSvc,
	}, nil
}

// seedFreeAccounts auto-creates a default account for providers that are free
// and require no credentials, so they are immediately usable without a manual
// "connect" step in the dashboard. Seedable providers are derived from the
// catalog: AuthKind "none", not hidden, and not a local-only provider (must
// have a non-localhost base URL that actually points at a remote service).
func seedFreeAccounts(ctx context.Context, accounts *persistence.AccountRepo, log *logrus.Logger) {
	// Local-only providers that should never be auto-seeded because they
	// depend on software running on the user's machine.
	localOnly := map[string]bool{
		"ollama-local": true,
		"vllm":         true,
		"sdwebui":      true,
		"comfyui":      true,
		"searxng":      true,
		"coqui":        true,
		"tortoise":     true,
		"google-tts":   true,
		"edge-tts":     true,
		"local-device": true,
	}
	for _, spec := range connectors.Catalog() {
		if spec.AuthKind != "none" || spec.Hidden || localOnly[spec.ID] {
			continue
		}
		existing, err := accounts.ListByProvider(ctx, schema.DefaultTenantID, spec.ID)
		if err != nil {
			log.Warn("seed free accounts: list failed", "provider", spec.ID, "err", err)
			continue
		}
		if len(existing) > 0 {
			// Clear any lingering cooldowns from a previous session so the
			// account is immediately usable after restart.
			if err := accounts.ClearProviderCooldowns(ctx, schema.DefaultTenantID, spec.ID); err != nil {
				log.Warn("seed free accounts: clear cooldowns failed", "provider", spec.ID, "err", err)
			}
			continue
		}
		now := time.Now()
		acc := schema.Account{
			ID:        uuid.NewString(),
			TenantID:  schema.DefaultTenantID,
			Provider:  spec.ID,
			Label:     spec.DisplayName + " (auto)",
			AuthKind:  string(schema.AuthNone),
			Priority:  100,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := accounts.Create(ctx, acc); err != nil {
			log.Warn("seed free accounts: create failed", "provider", spec.ID, "err", err)
			continue
		}
		log.Info("auto-seeded free provider account", "provider", spec.ID, "label", acc.Label)
	}
}

// Run starts the HTTP server and blocks until ctx is canceled, then shuts down
// gracefully.
func (a *App) Run(ctx context.Context) error {
	if a.meter != nil {
		a.meter.StartAsync(ctx)
	}
	if a.healthChecker != nil {
		a.bg.Add(1)
		go func() {
			defer a.bg.Done()
			a.healthChecker.Run(ctx, schema.DefaultTenantID)
		}()
	}

	// Background cooldown sweeper: periodically clears expired cooldowns so
	// accounts recover automatically without a restart.
	if a.accounts != nil {
		a.bg.Add(1)
		go func() {
			defer a.bg.Done()
			a.runCooldownSweeper(ctx)
		}()
	}

	// WASM hot reload: poll extension directory for .wasm file changes.
	if a.wasmEngine != nil && len(a.wasmModules) > 0 {
		a.bg.Add(1)
		go func() {
			defer a.bg.Done()
			log := a.log.WithField("component", "hot-reload")
			reloader := wasm.NewHotReloader(a.wasmEngine, a.cfg.WASM.ExtDir, a.wasmModules, a.cfg.WASM.HotReloadInterval, log)
			reloader.Run(ctx)
		}()
	}

	// Dynamic catalog sync: background worker for models.dev catalog and pricing.
	if a.catalogSvc != nil && a.cfg.Catalog.Enabled {
		a.bg.Add(1)
		go func() {
			defer a.bg.Done()
			a.catalogSvc.Start(ctx)
		}()
		go func() {
			if err := a.catalogSvc.SyncNow(context.Background()); err != nil {
				a.log.WithError(err).Debug("initial catalog sync failed")
			}
		}()
	}

	errCh := make(chan error, 1)

	listeners, err := a.loopbackListeners()
	if err != nil {
		return err
	}

	for _, ln := range listeners {
		ln := ln
		go func() {
			a.log.WithFields(logrus.Fields{"addr": ln.Addr().String(), "db": a.cfg.Database.Driver}).Info("FlameGate listening")
			if serveErr := a.server.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
				errCh <- serveErr
			}
		}()
	}

	if a.proxyServer != nil {
		go func() {
			addr := a.proxyServer.Addr
			a.log.WithFields(logrus.Fields{"addr": addr, "surface": "v1"}).Info("FlameGate proxy listening")
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				errCh <- fmt.Errorf("app: listen proxy %s: %w", addr, err)
				return
			}
			if serveErr := a.proxyServer.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
				errCh <- serveErr
			}
		}()
	}

	select {
	case <-ctx.Done():
		a.log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if a.proxyServer != nil {
			if err := a.proxyServer.Shutdown(shutdownCtx); err != nil {
				return err
			}
		}
		// Wait for background workers that query the DB (health
		// checker, cooldown sweeper) to return before closing the store. They
		// observe ctx cancellation and exit promptly; the bounded wait prevents
		// a slow in-flight worker from hanging shutdown while still closing the
		// use-after-close window against db.Close() (a pure-Go SQLite handle can
		// crash if a query races a Close).
		a.waitForBackground(5 * time.Second)
		// Drain pending guardrail audit and usage rows before closing the DB.
		if a.guardrailRetention != nil {
			a.guardrailRetention.Stop(2 * time.Second)
		}
		if a.guardrailAudit != nil {
			a.guardrailAudit.Stop(5 * time.Second)
		}
		if a.meter != nil {
			a.meter.Close(a.cfg.Meter.ShutdownFlushTimeout)
		}
		return a.db.Close()
	case err := <-errCh:
		return err
	}
}

// loopbackListeners opens the TCP listeners the server serves on. When the
// configured host is a loopback name ("localhost"), it binds both 127.0.0.1
// and ::1 so callbacks resolve regardless of whether the OS prefers IPv4 or
// IPv6. This matters on Windows, where "localhost" usually resolves to ::1
// first — a single 127.0.0.1 listener would leave OAuth callbacks to
// http://localhost hanging. For any explicit address, a single listener on
// that address is used.
func (a *App) loopbackListeners() ([]net.Listener, error) {
	host := a.cfg.Server.Host
	port := a.cfg.Server.Port

	if host == "localhost" {
		var lns []net.Listener
		for _, ip := range []string{"127.0.0.1", "::1"} {
			addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				// ::1 may be unavailable on hosts with IPv6 disabled. Tolerate
				// that as long as at least one listener came up.
				if ip == "::1" && len(lns) > 0 {
					a.log.Warn("IPv6 loopback unavailable; serving IPv4 only", "err", err)
					continue
				}
				for _, opened := range lns {
					_ = opened.Close()
				}
				return nil, fmt.Errorf("app: listen %s: %w", addr, err)
			}
			lns = append(lns, ln)
		}
		return lns, nil
	}

	ln, err := net.Listen("tcp", a.cfg.Addr())
	if err != nil {
		return nil, fmt.Errorf("app: listen %s: %w", a.cfg.Addr(), err)
	}
	return []net.Listener{ln}, nil
}

// ResolveDataDir returns the configured data directory, creating it if needed.
func ResolveDataDir(cfg config.Config) (string, error) {
	dir := cfg.Data.Dir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("app: resolve home dir: %w", err)
		}
		dir = filepath.Join(home, ".flamegate")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("app: create data dir %q: %w", dir, err)
	}
	return dir, nil
}

func LoadOrCreateMasterKey(cfg config.Config, dataDir string, log *logrus.Logger) ([]byte, error) {
	if cfg.Security.MasterKey != "" {
		key, err := crypto.DecodeMasterKey(cfg.Security.MasterKey)
		if err != nil {
			return nil, fmt.Errorf("app: invalid configured master key: %w", err)
		}
		return key, nil
	}

	keyPath := filepath.Join(dataDir, "master.key")
	if data, err := os.ReadFile(keyPath); err == nil {
		key, derr := crypto.DecodeMasterKey(string(data))
		if derr != nil {
			return nil, fmt.Errorf("app: corrupt master key file %q: %w", keyPath, derr)
		}
		return key, nil
	}

	key, err := crypto.GenerateMasterKey()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, []byte(crypto.EncodeMasterKey(key)), 0o600); err != nil {
		return nil, fmt.Errorf("app: persist master key: %w", err)
	}
	log.Warn("generated new master key", "path", keyPath,
		"note", "back this up; losing it makes stored credentials unrecoverable")
	return key, nil
}

// loadCustomProviders registers user-defined dynamic provider instances and
// their custom models from the database into the in-memory connector catalog,
// so they participate in discovery and routing immediately at startup.
func loadCustomProviders(ctx context.Context, db *persistence.DB, log *logrus.Logger) {
	repo := db.CustomProviders()
	providers, err := repo.ListProviders(ctx, schema.DefaultTenantID)
	if err != nil {
		log.Warn("load custom providers failed", "err", err)
		return
	}
	for _, p := range providers {
		dialect := core.DialectOpenAI
		if p.Dialect == string(core.DialectAnthropic) {
			dialect = core.DialectAnthropic
		}
		connectors.RegisterDynamicProvider(connectors.DynamicProvider{
			ID: p.ID, DisplayName: p.DisplayName, Alias: p.Alias, Dialect: dialect, BaseURL: p.BaseURL,
		})
	}

	models, err := repo.ListModels(ctx, schema.DefaultTenantID)
	if err != nil {
		log.Warn("load custom models failed", "err", err)
		return
	}
	byProvider := map[string][]connectors.ModelSpec{}
	for _, m := range models {
		kind := core.ServiceKind(m.Kind)
		if kind == "" {
			kind = core.ServiceLLM
		}
		byProvider[m.ProviderID] = append(byProvider[m.ProviderID], connectors.ModelSpec{
			ID: m.ModelID, Name: m.DisplayName, Kind: kind,
		})
	}
	for providerID, specs := range byProvider {
		connectors.SetDynamicModels(providerID, specs)
	}
	if len(providers) > 0 || len(models) > 0 {
		log.Info("loaded custom providers", "providers", len(providers), "models", len(models))
	}
}

// buildPricing projects the connector catalog into a meter pricing table.
func buildPricing() map[string]meter.Price {

	specs := connectors.Catalog()
	prices := make([]meter.SpecPrice, 0, len(specs))
	for _, s := range specs {
		prices = append(prices, meter.SpecPrice{ID: s.ID, InputPerM: s.InputPerM, OutputPerM: s.OutputPerM})
	}
	return meter.PricingFromCatalog(prices)
}

// cooldownSweepInterval is how often the background sweeper checks for expired
// cooldowns. Short enough to recover within seconds; long enough to be trivial.
const cooldownSweepInterval = 15 * time.Second

// waitForBackground blocks until all tracked background workers have returned
// or the timeout elapses, whichever comes first. Bounding the wait keeps
// shutdown responsive even if a worker is stuck in a slow network call.
func (a *App) waitForBackground(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		a.bg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		a.log.Warn("background workers did not stop within timeout; proceeding with shutdown")
	}
}

// runCooldownSweeper periodically clears expired cooldowns so accounts auto-
// recover after rate-limit / auth errors without requiring a restart.
func (a *App) runCooldownSweeper(ctx context.Context) {
	ticker := time.NewTicker(cooldownSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := a.accounts.ClearExpiredCooldowns(ctx)
			if err != nil {
				a.log.Debug("cooldown sweep failed", "err", err)
			} else if n > 0 {
				a.log.Info("sweeper cleared expired cooldowns", "count", n)
			}
		}
	}
}

// buildModelPrices returns an empty per-model pricing table.
// Static model_prices.go was purged; meter records CostMicros=0 until pricing returns.
func buildModelPrices() map[string]meter.Price {
	return map[string]meter.Price{}
}
// firstAuthModeOf returns the first declared auth mode (oauth preferred), or
// api_key as the conservative default. Mirrors admin.firstAuthMode.
func firstAuthModeOf(modes []string) string {
	for _, m := range modes {
		switch m {
		case "oauth", "api_key", "none":
			return m
		}
	}
	return "api_key"
}
