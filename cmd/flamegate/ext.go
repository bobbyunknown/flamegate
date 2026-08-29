package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/app"
	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/extstore"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
	"github.com/bobbyunknown/flamegate/internal/shared/crypto"
	"github.com/bobbyunknown/flamegate/internal/shared/vault"
)

// extDeps holds CLI-scoped dependencies for extension subcommands.
type extDeps struct {
	cfg     config.Config
	db      *persistence.DB
	engine  *wasm.Engine
	vault   *vault.Vault
	modules map[string]*wasm.Module
	store   *extstore.Installer
	log     *logrus.Logger
}

// initExtensionCLI opens DB, vault, and WASM engine without starting the server.
func initExtensionCLI(configPath string) (*extDeps, error) {
	cfg, log, _, err := setup(configPath)
	if err != nil {
		return nil, err
	}

	dataDir, err := app.ResolveDataDir(cfg)
	if err != nil {
		return nil, fmt.Errorf("ext: resolve data dir: %w", err)
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
		return nil, fmt.Errorf("ext: open db: %w", err)
	}
	if err := db.Migrate(); err != nil {
		return nil, fmt.Errorf("ext: migrate: %w", err)
	}

	masterKey, err := app.LoadOrCreateMasterKey(cfg, dataDir, log)
	if err != nil {
		return nil, fmt.Errorf("ext: load master key: %w", err)
	}
	sealer, err := crypto.NewSealer(masterKey)
	if err != nil {
		return nil, fmt.Errorf("ext: build sealer: %w", err)
	}
	v := vault.New(sealer)

	engine := wasm.NewEngine(cfg.WASM, v, db.Accounts(), &http.Client{Timeout: 60 * time.Second})
	modules := map[string]*wasm.Module{}

	if exts, scanErr := wasm.Scan(cfg.WASM.ExtDir); scanErr == nil {
		for _, ext := range exts {
			if compileErr := engine.Compile(context.Background(), ext.Slug, ext.WasmBytes, wasm.ExtensionConfig{
				Slug:         ext.Slug,
				Timeout:      time.Duration(ext.Schema.Timeout) * time.Second,
				AllowedHosts: cfg.WASM.AllowedHosts,
				Entrypoints:  ext.Schema.Entrypoints,
			}); compileErr != nil {
				log.WithError(compileErr).Warn("ext: compile failed", "slug", ext.Slug)
				continue
			}
			modules[ext.Slug] = wasm.NewModule(engine, ext.Slug, cfg.WASM.MaxInst)
		}
	}

	return &extDeps{
		cfg:     cfg,
		db:      db,
		engine:  engine,
		vault:   v,
		modules: modules,
		store:   extstore.NewInstallerFromConfig(cfg.WASM, &http.Client{Timeout: 60 * time.Second}, engine, db.Extensions()),
		log:     log,
	}, nil
}

// cmdExt dispatches to extension sub-subcommands.
func cmdExt(args []string) error {
	if len(args) == 0 {
		printExtUsage(os.Stderr)
		return errSilent
	}

	sub := args[0]
	rest := args[1:]

	switch sub {
	case "install":
		return cmdExtInstall(rest)
	case "uninstall":
		return cmdExtUninstall(rest)
	case "list":
		return cmdExtList(rest)
	case "enable":
		return cmdExtEnable(rest)
	case "disable":
		return cmdExtDisable(rest)
	case "update":
		return cmdExtUpdate(rest)
	case "store":
		return cmdExtStore(rest)
	case "help", "-h", "--help":
		printExtUsage(os.Stdout)
		return nil
	default:
		printExtUsage(os.Stderr)
		return fmt.Errorf("unknown ext subcommand %q", sub)
	}
}

func printExtUsage(w *os.File) {
	fmt.Fprint(w, `Usage: flamegate ext <command> [flags]

Commands:
  install <source>   Install an extension from a directory (schema.json + .wasm),
                     OR a remote source: store:<slug>, github:owner/repo@ref, or
                     url:https://.../x.zip
  uninstall <slug>   Remove an installed extension
  list               List all installed extensions
  enable <slug>      Enable a disabled extension
  disable <slug>     Disable an active extension
  update <slug>      Upgrade an installed extension to its latest release
  store list         List the extension store catalog
  store search <q>   Search the store catalog
  help               Show this help

Flags:
  -c, --config <path>  Path to a config file (optional)
`)
}

// cmdExtInstall installs from a local directory or a remote source
// (store://, github:, url:). Local dirs use the legacy path; remote sources go
// through the unified extstore pipeline.
func cmdExtInstall(args []string) error {
	fs := flag.NewFlagSet("ext install", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: flamegate ext install <path|store:slug|github:o/r@ref|url:...>")
	}

	source := fs.Arg(0)
	// Route bare slugs (store) and github:/url: sources through the unified
	// pipeline; only explicit local paths keep the legacy directory install.
	spec, _ := extstore.ParseSource(source)
	if spec.Kind != extstore.SourceLocal {
		deps, err := initExtensionCLI(configVal(fs, configPath))
		if err != nil {
			return err
		}
		defer deps.db.SQL().Close()
		defer deps.engine.Close()

		res, err := deps.store.Install(context.Background(), source)
		if err != nil {
			return fmt.Errorf("ext: install remote: %w", err)
		}
		fmt.Printf("Extension %q installed successfully (version %s, trust %s, checksum %s)\n",
			res.Slug, res.Version, res.Trust, res.Checksum)
		return nil
	}

	dir := source
	deps, err := initExtensionCLI(configVal(fs, configPath))
	if err != nil {
		return err
	}
	defer deps.db.SQL().Close()
	defer deps.engine.Close()

	ctx := context.Background()

	schemaPath := filepath.Join(dir, "schema.json")
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("ext: read schema.json from %s: %w", dir, err)
	}

	var extSchema wasm.ExtensionSchema
	if err := json.Unmarshal(schemaData, &extSchema); err != nil {
		return fmt.Errorf("ext: parse schema.json: %w", err)
	}

	slug := extSchema.Slug
	if slug == "" {
		return fmt.Errorf("ext: schema.json missing 'slug'")
	}
	if !wasm.IsValidSlug(slug) {
		return fmt.Errorf("ext: invalid slug format: %q", slug)
	}
	if connectors.IsNativeSlug(slug) {
		return fmt.Errorf("ext: slug %q is a native provider", slug)
	}

	if _, err := deps.db.Extensions().FindBySlug(ctx, slug); err == nil {
		return fmt.Errorf("ext: extension %q already installed", slug)
	}

	// Find .wasm file.
	wasmPath := filepath.Join(dir, slug+".wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		wasmPath = filepath.Join(dir, "dist", slug+".wasm")
		wasmBytes, err = os.ReadFile(wasmPath)
	}
	if err != nil {
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			return fmt.Errorf("ext: read wasm from %s: %w", dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".wasm" {
				wasmPath = filepath.Join(dir, e.Name())
				wasmBytes, err = os.ReadFile(wasmPath)
				if err == nil {
					break
				}
			}
		}
		if err != nil {
			// Check dist/ subfolder.
			if distEntries, distErr := os.ReadDir(filepath.Join(dir, "dist")); distErr == nil {
				for _, e := range distEntries {
					if !e.IsDir() && filepath.Ext(e.Name()) == ".wasm" {
						wasmPath = filepath.Join(dir, "dist", e.Name())
						wasmBytes, err = os.ReadFile(wasmPath)
						if err == nil {
							break
						}
					}
				}
			}
		}
		if err != nil {
			return fmt.Errorf("ext: no .wasm file found in %s", dir)
		}
	}

	extCfg := wasm.ExtensionConfig{
		Slug:         slug,
		Timeout:      time.Duration(extSchema.Timeout) * time.Second,
		AllowedHosts: deps.cfg.WASM.AllowedHosts,
		Entrypoints:  extSchema.Entrypoints,
	}

	if err := deps.engine.Compile(ctx, slug, wasmBytes, extCfg); err != nil {
		return fmt.Errorf("ext: compile: %w", err)
	}

	extDir := deps.cfg.WASM.ExtDir
	slugDir := filepath.Join(extDir, slug)
	if err := os.MkdirAll(slugDir, 0o755); err != nil {
		return fmt.Errorf("ext: create directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(slugDir, slug+".wasm"), wasmBytes, 0o644); err != nil {
		return fmt.Errorf("ext: write wasm: %w", err)
	}
	if err := os.WriteFile(filepath.Join(slugDir, "schema.json"), schemaData, 0o644); err != nil {
		return fmt.Errorf("ext: write schema: %w", err)
	}

	now := time.Now()
	ext := schema.Extension{
		ID:           slug,
		TenantID:     schema.DefaultTenantID,
		Slug:         slug,
		Name:         extSchema.Name,
		Version:      extSchema.Version,
		Description:  extSchema.Description,
		WasmPath:     wasmPath,
		SchemaPath:   schemaPath,
		State:        "ACTIVE",
		Capabilities: mustJSONExt(extSchema.Entrypoints),
		Entrypoints:  mustJSONExt(extSchema.Entrypoints),
		Config:       mustJSONExt(map[string]any{"timeout": extSchema.Timeout, "max_instances": extSchema.MaxInstances}),
		CompiledAt:   new(time.Now()),
		InstalledAt:  now,
		UpdatedAt:    now,
	}

	if err := deps.db.Extensions().Create(ctx, ext); err != nil {
		return fmt.Errorf("ext: save to database: %w", err)
	}

	fmt.Printf("Extension %q installed successfully (version %s)\n", slug, extSchema.Version)
	return nil
}

// cmdExtUninstall removes an extension.
func cmdExtUninstall(args []string) error {
	fs := flag.NewFlagSet("ext uninstall", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: flamegate ext uninstall <slug>")
	}
	slug := fs.Arg(0)

	deps, err := initExtensionCLI(configVal(fs, configPath))
	if err != nil {
		return err
	}
	defer deps.db.SQL().Close()
	defer deps.engine.Close()

	ctx := context.Background()
	ext, err := deps.db.Extensions().FindBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("ext: extension %q not found", slug)
	}

	_ = deps.engine.Unload(slug)

	if err := deps.db.ExtensionModels().DeleteByExtension(ctx, ext.ID); err != nil {
		deps.log.WithError(err).Warn("ext: failed to delete models", "slug", slug)
	}

	if err := deps.db.Extensions().Delete(ctx, ext.ID); err != nil {
		return fmt.Errorf("ext: delete from database: %w", err)
	}

	if ext.WasmPath != "" {
		dir := filepath.Dir(ext.WasmPath)
		_ = os.RemoveAll(dir)
	}

	fmt.Printf("Extension %q uninstalled\n", slug)
	return nil
}

// cmdExtList prints all installed extensions in a table.
func cmdExtList(args []string) error {
	fs := flag.NewFlagSet("ext list", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	deps, err := initExtensionCLI(configVal(fs, configPath))
	if err != nil {
		return err
	}
	defer deps.db.SQL().Close()
	defer deps.engine.Close()

	ctx := context.Background()
	exts, err := deps.db.Extensions().List(ctx, schema.DefaultTenantID)
	if err != nil {
		return fmt.Errorf("ext: list extensions: %w", err)
	}

	if len(exts) == 0 {
		fmt.Println("No extensions installed.")
		return nil
	}

	fmt.Printf("%-20s %-12s %-10s %-8s %s\n", "SLUG", "VERSION", "STATE", "MODELS", "NAME")
	fmt.Printf("%-20s %-12s %-10s %-8s %s\n", "----", "-------", "-----", "------", "----")

	for _, ext := range exts {
		models, _ := deps.db.ExtensionModels().ListByExtension(ctx, ext.ID)
		fmt.Printf("%-20s %-12s %-10s %-8d %s\n", ext.Slug, ext.Version, ext.State, len(models), ext.Name)
	}

	return nil
}

// cmdExtEnable transitions an extension to ACTIVE.
func cmdExtEnable(args []string) error {
	return cmdExtSetState(args, "ACTIVE")
}

// cmdExtDisable transitions an extension to DISABLED.
func cmdExtDisable(args []string) error {
	return cmdExtSetState(args, "DISABLED")
}

func cmdExtSetState(args []string, state string) error {
	fs := flag.NewFlagSet("ext state", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: flamegate ext %s <slug>", state)
	}
	slug := fs.Arg(0)

	deps, err := initExtensionCLI(configVal(fs, configPath))
	if err != nil {
		return err
	}
	defer deps.db.SQL().Close()
	defer deps.engine.Close()

	ctx := context.Background()
	ext, err := deps.db.Extensions().FindBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("ext: extension %q not found", slug)
	}

	if ext.State == state {
		fmt.Printf("Extension %q is already %s\n", slug, state)
		return nil
	}

	mod, ok := deps.modules[slug]
	if !ok {
		mod = wasm.NewModule(deps.engine, slug, deps.cfg.WASM.MaxInst)
		deps.modules[slug] = mod
	}

	var targetState int32
	if state == "ACTIVE" {
		targetState = wasm.StateActive
	} else {
		targetState = wasm.StateDisabled
	}

	log := deps.log.WithField("slug", slug)
	if err := wasm.Transition(mod, targetState, log); err != nil {
		return fmt.Errorf("ext: transition failed: %w", err)
	}

	ext.State = state
	ext.UpdatedAt = time.Now()
	if err := deps.db.Extensions().Update(ctx, ext); err != nil {
		return fmt.Errorf("ext: update database: %w", err)
	}

	fmt.Printf("Extension %q is now %s\n", slug, state)
	return nil
}

func mustJSONExt(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// isRemoteSource reports whether an install argument is a store/github/url
// source rather than a local directory path.
func isRemoteSource(s string) bool {
	if s == "" {
		return false
	}
	return hasPrefixAny(s, []string{"store:", "github:", "url:", "https://", "http://"})
}

func hasPrefixAny(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// cmdExtUpdate upgrades an installed extension to its latest release.
func cmdExtUpdate(args []string) error {
	fs := flag.NewFlagSet("ext update", flag.ContinueOnError)
	configPath := resolveConfigFlag(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: flamegate ext update <slug>")
	}
	slug := fs.Arg(0)

	deps, err := initExtensionCLI(configVal(fs, configPath))
	if err != nil {
		return err
	}
	defer deps.db.SQL().Close()
	defer deps.engine.Close()

	ctx := context.Background()
	ext, err := deps.db.Extensions().FindBySlug(ctx, slug)
	if err != nil {
		return fmt.Errorf("ext: extension %q not found", slug)
	}
	if ext.SourceURI == "" {
		return fmt.Errorf("ext: extension %q was installed locally; update only works for store/github/url sources", slug)
	}

	res, err := deps.store.Install(ctx, ext.SourceURI)
	if err != nil {
		return fmt.Errorf("ext: update %q: %w", slug, err)
	}
	fmt.Printf("Extension %q updated to %s (trust %s, checksum %s)\n", res.Slug, res.Version, res.Trust, res.Checksum)
	return nil
}

// cmdExtStore implements `store list` and `store search`.
func cmdExtStore(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: flamegate ext store <list|search <q>>")
	}
	var storeList func(deps *extDeps) error
	switch args[0] {
	case "list":
		storeList = storeListAll
	case "search":
		if len(args) < 2 {
			return fmt.Errorf("usage: flamegate ext store search <query>")
		}
		q := args[1]
		storeList = func(deps *extDeps) error { return storeSearch(deps, q) }
	default:
		return fmt.Errorf("unknown store subcommand %q", args[0])
	}

	deps, err := initExtensionCLI("")
	if err != nil {
		return err
	}
	defer deps.db.SQL().Close()
	defer deps.engine.Close()

	return storeList(deps)
}

func storeListAll(deps *extDeps) error {
	items, err := deps.store.ListStore(context.Background())
	if err != nil {
		return fmt.Errorf("ext: store list: %w", err)
	}
	if len(items) == 0 {
		fmt.Println("No extensions in the store catalog.")
		return nil
	}
	fmt.Printf("%-24s %-12s %s\n", "SLUG", "VERSION", "NAME")
	fmt.Printf("%-24s %-12s %s\n", "----", "-------", "----")
	for _, it := range items {
		fmt.Printf("%-24s %-12s %s\n", it.Slug, it.Version, it.Name)
	}
	return nil
}

func storeSearch(deps *extDeps, q string) error {
	items, err := deps.store.ListStore(context.Background())
	if err != nil {
		return fmt.Errorf("ext: store search: %w", err)
	}
	q = strings.ToLower(strings.TrimSpace(q))
	found := 0
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Slug), q) || strings.Contains(strings.ToLower(it.Name), q) {
			fmt.Printf("%-24s %-12s %s\n", it.Slug, it.Version, it.Name)
			found++
		}
	}
	if found == 0 {
		fmt.Printf("No matches in store for %q.\n", q)
	}
	return nil
}
