package extstore

import (
	"net/http"
	"os"
	"time"

	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
)

// wasmCfgAdapter adapts config.WASMConfig to the hostConfig interface used by
// the installer, plus the optional index-URL method.
type wasmCfgAdapter struct{ c config.WASMConfig }

func (a wasmCfgAdapter) ExtDir() string { return a.c.ExtDir }

func (a wasmCfgAdapter) StoreIndexURL() string { return a.c.StoreIndexURL }

// NewInstallerFromConfig builds an Installer wired from WASMConfig and the
// runtime deps, suitable for both the server (app.Build) and the CLI. engine
// may be nil when only resolution/verification flows are exercised.
func NewInstallerFromConfig(
	wasmCfg config.WASMConfig,
	httpc *http.Client,
	engine *wasm.Engine,
	repo ExtensionRepo,
) *Installer {
	cache := NewTTLCache()
	return NewInstaller(
		wasmCfgAdapter{c: wasmCfg},
		NewGithubClient(httpc, tokenFunc(wasmCfg), cache, ttl(wasmCfg)),
		NewIndexStore(httpc, cache),
		NewDownloader(httpc),
		engine,
		repo,
		wasmCfg.ExtPublicKeys,
		wasmCfg.AllowUnsigned,
		defaultExtConfBuilder(wasmCfg),
	)
}

// tokenFunc reads GITHUB_TOKEN (or configured env name) at request time.
func tokenFunc(c config.WASMConfig) func() string {
	name := c.GithubTokenEnv
	if name == "" {
		name = "GITHUB_TOKEN"
	}
	return func() string { return os.Getenv(name) }
}

// ttl returns StoreCacheTTL bounded to a sane floor.
func ttl(c config.WASMConfig) time.Duration {
	if c.StoreCacheTTL < 5*time.Second {
		return 10 * time.Minute
	}
	return c.StoreCacheTTL
}

// defaultExtConfBuilder maps schema.Entrypoints + host knobs into
// wasm.ExtensionConfig, mirroring how app.go compiles installed extensions.
func defaultExtConfBuilder(cfg config.WASMConfig) func(schema.Extension, map[string]string) wasm.ExtensionConfig {
	return func(stub schema.Extension, entrypoints map[string]string) wasm.ExtensionConfig {
		timeout := cfg.DefaultTimeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		ec := wasm.ExtensionConfig{
			Slug:         stub.Slug,
			Timeout:      timeout,
			MaxMemoryMB:  cfg.MaxMemoryMB,
			AllowedHosts: cfg.AllowedHosts,
			Entrypoints:  entrypoints,
		}
		if stub.DefaultAccountKey != "" {
			ec.AccountKey = stub.DefaultAccountKey
		}
		return ec
	}
}