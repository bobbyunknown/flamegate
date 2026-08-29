// Package wasm implements the FlameGate WASM extension engine using wazero.
package wasm

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/shared/observ"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// ExtensionConfig holds per-extension runtime configuration parsed from schema.json.
type ExtensionConfig struct {
	Slug         string
	Timeout      time.Duration
	MaxMemoryMB  int
	AllowedHosts []string
	Entrypoints  map[string]string
	AccountKey   string
}

// CompiledModule holds a compiled WASM module and its config.
type CompiledModule struct {
	compiled wazero.CompiledModule
	config   ExtensionConfig
}

// Engine manages compiled WASM modules and creates instances on demand.
// It is safe for concurrent use.
type Engine struct {
	runtime     wazero.Runtime
	modules     map[string]CompiledModule
	instMu      map[string]*sync.Mutex
	mu          sync.RWMutex
	cfg         config.WASMConfig
	instCount   atomic.Int64
	vault       *vault.Vault
	accountRepo ports.AccountRepository
	httpClient  *http.Client
	metrics     *observ.Metrics // nil = no metrics
}

// NewEngine creates a new WASM engine with the given config and dependencies.
func NewEngine(cfg config.WASMConfig, v *vault.Vault, accountRepo ports.AccountRepository, httpClient *http.Client) *Engine {
	r := wazero.NewRuntime(context.Background())
	wasi_snapshot_preview1.MustInstantiate(context.Background(), r)
	return &Engine{
		runtime:     r,
		modules:     make(map[string]CompiledModule),
		instMu:      make(map[string]*sync.Mutex),
		cfg:         cfg,
		vault:       v,
		accountRepo: accountRepo,
		httpClient:  httpClient,
	}
}

// SetMetrics attaches a Prometheus metrics collector to the engine.
func (e *Engine) SetMetrics(m *observ.Metrics) { e.metrics = m }

// Compile compiles a WASM module from raw bytes and stores it by slug.
func (e *Engine) Compile(ctx context.Context, slug string, wasm []byte, extCfg ExtensionConfig) error {
	start := time.Now()
	compiled, err := e.runtime.CompileModule(ctx, wasm)
	if err != nil {
		return fmt.Errorf("compile module %s: %w", slug, err)
	}

	if e.metrics != nil {
		e.metrics.WASMCompileDuration.Observe(time.Since(start).Seconds())
	}

	e.mu.Lock()
	e.modules[slug] = CompiledModule{compiled: compiled, config: extCfg}
	e.mu.Unlock()
	return nil
}

// Get returns the compiled module for a slug, or (nil, false) if not found.
func (e *Engine) Get(slug string) (wazero.CompiledModule, bool) {
	e.mu.RLock()
	cm, ok := e.modules[slug]
	e.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return cm.compiled, true
}

// GetConfig returns the extension config for a slug.
func (e *Engine) GetConfig(slug string) (ExtensionConfig, bool) {
	e.mu.RLock()
	cm, ok := e.modules[slug]
	e.mu.RUnlock()
	return cm.config, ok
}

// Unload removes a compiled module by slug and closes it.
func (e *Engine) Unload(slug string) error {
	e.mu.Lock()
	cm, ok := e.modules[slug]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("wasm: module %s not found", slug)
	}
	delete(e.modules, slug)
	e.mu.Unlock()

	return cm.compiled.Close(context.Background())
}

// GetConnector returns a WASMConnector for the given slug.
func (e *Engine) GetConnector(slug string) (*WASMConnector, bool) {
	e.mu.RLock()
	cm, ok := e.modules[slug]
	e.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return &WASMConnector{
		engine:   e,
		module:   NewModule(e, slug, e.cfg.MaxInst),
		slug:     slug,
		compiled: cm.compiled,
		cfg:      cm.config,
	}, true
}

// HasExtensions returns true if at least one module is compiled.
func (e *Engine) HasExtensions() bool {
	e.mu.RLock()
	n := len(e.modules)
	e.mu.RUnlock()
	return n > 0
}

// Slugs returns all compiled extension slugs.
func (e *Engine) Slugs() []string {
	e.mu.RLock()
	slugs := make([]string, 0, len(e.modules))
	for slug := range e.modules {
		slugs = append(slugs, slug)
	}
	e.mu.RUnlock()
	return slugs
}

// Instantiate creates a new module instance with the given host function environment.
// Caller MUST call Close() on the returned module when done.
func (e *Engine) Instantiate(ctx context.Context, slug string, env *InvokeEnv) (api.Module, error) {
	e.mu.RLock()
	cm, ok := e.modules[slug]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("wasm: module %s not found", slug)
	}

	instID := e.instCount.Add(1)

	// Serialize "env" host module creation per-slug.
	e.mu.Lock()
	if e.instMu[slug] == nil {
		e.instMu[slug] = &sync.Mutex{}
	}
	slugMu := e.instMu[slug]
	e.mu.Unlock()
	slugMu.Lock()
	defer slugMu.Unlock()

	// Host module MUST be named "env" to match guest #[link(wasm_import_module = "env")].
	hostModule := e.buildHostModule("env", env)
	hostMod, err := hostModule.Instantiate(ctx)
	if err != nil {
		return nil, fmt.Errorf("wasm: instantiate host module %s: %w", slug, err)
	}
	defer hostMod.Close(ctx)

	// Instantiate the guest module (imports resolved from "env" host).
	// If the module exports "_initialize" (standard WASI reactor / TinyGo), invoke that
	// instead of "_start" (which calls proc_exit(0) upon completion and closes the instance).
	modCfg := wazero.NewModuleConfig().WithName(fmt.Sprintf("fg-guest-%s-%d", slug, instID))
	if hasExport(cm.compiled, "_initialize") {
		modCfg = modCfg.WithStartFunctions("_initialize")
	} else if !hasExport(cm.compiled, "_start") {
		modCfg = modCfg.WithStartFunctions()
	}

	guestMod, err := e.runtime.InstantiateModule(ctx, cm.compiled, modCfg)
	if err != nil {
		return nil, fmt.Errorf("wasm: instantiate guest module %s: %w", slug, err)
	}

	return guestMod, nil
}

// Close releases all compiled modules and the wazero runtime.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for slug, cm := range e.modules {
		_ = cm.compiled.Close(context.Background())
		delete(e.modules, slug)
	}
	return e.runtime.Close(context.Background())
}

// DiscoveredModel represents a model returned by an extension's list_models entrypoint.
type DiscoveredModel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListModels calls the extension's list_models entrypoint and returns discovered models.
// The guest function calls get_credentials + http_post internally. Returns empty
// if the extension does not export list_models.
func (e *Engine) ListModels(ctx context.Context, slug string, creds core.Credentials) ([]DiscoveredModel, error) {
	e.mu.RLock()
	cm, ok := e.modules[slug]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("wasm: module %s not found", slug)
	}

	// Check for list_models entrypoint in config.
	entrypointName := cm.config.Entrypoints["models"]
	if entrypointName == "" {
		return nil, nil
	}

	// Check if the compiled module actually exports this function.
	if !hasExport(cm.compiled, entrypointName) {
		return nil, nil
	}

	// Create a fresh module with host functions.
	log := logrus.WithField("slug", slug)
	env := &InvokeEnv{
		Ctx:          ctx,
		Slug:         slug,
		Creds:        creds,
		Logger:       log,
		Vault:        e.vault,
		AccountRepo:  e.accountRepo,
		AllowedHosts: cm.config.AllowedHosts,
		HTTPClient:   e.httpClient,
	}

	inst, err := e.Instantiate(ctx, slug, env)
	if err != nil {
		return nil, fmt.Errorf("wasm: instantiate for list_models %s: %w", slug, err)
	}
	defer inst.Close(ctx)

	// Call list_models() — no args, returns resp_ptr.
	fn := inst.ExportedFunction(entrypointName)
	if fn == nil {
		return nil, nil
	}

	results, err := fn.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("wasm: list_models %s: %w", slug, err)
	}

	respPtr := uint32(results[0])
	if respPtr == 0 {
		return nil, nil
	}

	// Guest ABI: write_json returns ptr to [4-byte LE len][JSON] (same as invoke).
	var models []DiscoveredModel
	if err := readGuestJSON(inst, respPtr, 0, &models); err != nil {
		return nil, fmt.Errorf("wasm: parse list_models response %s: %w", slug, err)
	}

	return models, nil
}

// CallCapability relays an arbitrary guest call over the chat entrypoint with an
// extra capability. The guest owns all provider logic (OAuth URL building,
// token exchange, refresh); the host only relays. Returns nil for empty guest
// response; guest "error" is surfaced as a map entry.
func (e *Engine) CallCapability(ctx context.Context, slug, capability string, args map[string]any) (map[string]any, error) {
	e.mu.RLock()
	cm, ok := e.modules[slug]
	e.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("wasm: module %s not found", slug)
	}
	ep := cm.config.Entrypoints["chat"]
	if ep == "" {
		ep = "invoke"
	}
	if !hasExport(cm.compiled, ep) {
		return nil, fmt.Errorf("wasm: %s does not export %s", slug, ep)
	}

	log := logrus.WithField("slug", slug)
	env := &InvokeEnv{
		Ctx: ctx, Slug: slug, Logger: log,
		Vault: e.vault, AccountRepo: e.accountRepo,
		AllowedHosts: cm.config.AllowedHosts, HTTPClient: e.httpClient,
	}
	inst, err := e.Instantiate(ctx, slug, env)
	if err != nil {
		return nil, fmt.Errorf("wasm: instantiate %s: %w", slug, err)
	}
	defer inst.Close(ctx)

	fn := inst.ExportedFunction(ep)
	if fn == nil {
		return nil, fmt.Errorf("wasm: %s does not export %s", slug, ep)
	}
	req := guestRequest{Capability: capability, Extra: args}
	reqPtr, reqSize, err := writeGuestJSON(ctx, inst, req)
	if err != nil {
		return nil, fmt.Errorf("wasm: write request: %w", err)
	}
	defer func() { _ = deallocGuest(ctx, inst, reqPtr, reqSize) }()

	results, err := fn.Call(ctx, uint64(reqPtr), uint64(reqSize))
	if err != nil {
		return nil, fmt.Errorf("wasm: capability %s %s: %w", capability, slug, err)
	}
	respPtr := uint32(results[0])
	if respPtr == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := readGuestJSON(inst, respPtr, 0, &out); err != nil {
		return nil, fmt.Errorf("wasm: parse %s response %s: %w", capability, slug, err)
	}
	return out, nil
}

// hasExport checks if a compiled module exports a function with the given name.
func hasExport(compiled wazero.CompiledModule, name string) bool {
	_, ok := compiled.ExportedFunctions()[name]
	return ok
}
