package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAllowPrivateBaseURLFromEnv(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{
			name: "canonical env var",
			key:  "FLAMEGATE_SECURITY__ALLOW_PRIVATE_BASE_URL",
		},
		{
			name: "legacy missing underscore env var",
			key:  "FLAMEGATE_SECURITY__ALLOW_PRIVATE_BASEURL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetEnv(t, "FLAMEGATE_SECURITY__ALLOW_PRIVATE_BASE_URL")
			unsetEnv(t, "FLAMEGATE_SECURITY__ALLOW_PRIVATE_BASEURL")
			t.Setenv(tt.key, "true")

			cfg, err := Load("")
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
			if !cfg.Security.AllowPrivateBaseURL {
				t.Fatalf("AllowPrivateBaseURL = false, want true")
			}
		})
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()

	old, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func TestWASMConfigDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.WASM.ExtDir == "" {
		t.Fatalf("WASM.ExtDir default is empty, want non-empty")
	}
	if cfg.WASM.MaxMemoryMB != 16 {
		t.Fatalf("WASM.MaxMemoryMB = %d, want 16", cfg.WASM.MaxMemoryMB)
	}
	if cfg.WASM.MaxInst != 16 {
		t.Fatalf("WASM.MaxInst = %d, want 16", cfg.WASM.MaxInst)
	}
	if cfg.WASM.DefaultTimeout != 60*time.Second {
		t.Fatalf("WASM.DefaultTimeout = %v, want 60s", cfg.WASM.DefaultTimeout)
	}
	if cfg.WASM.HotReloadInterval != 10*time.Second {
		t.Fatalf("WASM.HotReloadInterval = %v, want 10s", cfg.WASM.HotReloadInterval)
	}
	if len(cfg.WASM.AllowedHosts) != 0 {
		t.Fatalf("WASM.AllowedHosts = %v, want empty (allow all)", cfg.WASM.AllowedHosts)
	}
	if cfg.WASM.ForceWasmAll {
		t.Fatalf("WASM.ForceWasmAll = %v, want false", cfg.WASM.ForceWasmAll)
	}
}

func TestWASMConfigEnvOverride(t *testing.T) {
	unsetEnv(t, "FLAMEGATE_WASM__EXT_DIR")
	unsetEnv(t, "FLAMEGATE_WASM__MAX_MEMORY_MB")
	unsetEnv(t, "FLAMEGATE_WASM__MAX_INST")
	unsetEnv(t, "FLAMEGATE_WASM__DEFAULT_TIMEOUT")
	unsetEnv(t, "FLAMEGATE_WASM__HOT_RELOAD_INTERVAL")
	unsetEnv(t, "FLAMEGATE_WASM__FORCE_WASM_ALL")
	t.Setenv("FLAMEGATE_WASM__EXT_DIR", "/tmp/test-exts")
	t.Setenv("FLAMEGATE_WASM__MAX_MEMORY_MB", "32")
	t.Setenv("FLAMEGATE_WASM__MAX_INST", "8")
	t.Setenv("FLAMEGATE_WASM__DEFAULT_TIMEOUT", "120s")
	t.Setenv("FLAMEGATE_WASM__HOT_RELOAD_INTERVAL", "5s")
	t.Setenv("FLAMEGATE_WASM__FORCE_WASM_ALL", "true")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.WASM.ExtDir != "/tmp/test-exts" {
		t.Fatalf("WASM.ExtDir = %q, want /tmp/test-exts", cfg.WASM.ExtDir)
	}
	if cfg.WASM.MaxMemoryMB != 32 {
		t.Fatalf("WASM.MaxMemoryMB = %d, want 32", cfg.WASM.MaxMemoryMB)
	}
	if cfg.WASM.MaxInst != 8 {
		t.Fatalf("WASM.MaxInst = %d, want 8", cfg.WASM.MaxInst)
	}
	if cfg.WASM.DefaultTimeout != 120*time.Second {
		t.Fatalf("WASM.DefaultTimeout = %v, want 120s", cfg.WASM.DefaultTimeout)
	}
	if cfg.WASM.HotReloadInterval != 5*time.Second {
		t.Fatalf("WASM.HotReloadInterval = %v, want 5s", cfg.WASM.HotReloadInterval)
	}
	if !cfg.WASM.ForceWasmAll {
		t.Fatalf("WASM.ForceWasmAll = %v, want true", cfg.WASM.ForceWasmAll)
	}
}

func TestWASMConfigEnvOverrideAllowedHosts(t *testing.T) {
	unsetEnv(t, "FLAMEGATE_WASM__ALLOWED_HOSTS")
	t.Setenv("FLAMEGATE_WASM__ALLOWED_HOSTS", "api.openai.com,api.anthropic.com")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.WASM.AllowedHosts) != 2 {
		t.Fatalf("WASM.AllowedHosts len = %d, want 2; got %v", len(cfg.WASM.AllowedHosts), cfg.WASM.AllowedHosts)
	}
	if cfg.WASM.AllowedHosts[0] != "api.openai.com" {
		t.Fatalf("WASM.AllowedHosts[0] = %q, want api.openai.com", cfg.WASM.AllowedHosts[0])
	}
	if cfg.WASM.AllowedHosts[1] != "api.anthropic.com" {
		t.Fatalf("WASM.AllowedHosts[1] = %q, want api.anthropic.com", cfg.WASM.AllowedHosts[1])
	}
}

func TestWASMConfigStoreDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !cfg.WASM.AllowUnsigned {
		t.Fatalf("WASM.AllowUnsigned = %v, want true (tiered default)", cfg.WASM.AllowUnsigned)
	}
	if cfg.WASM.StoreIndexURL == "" || !strings.Contains(cfg.WASM.StoreIndexURL, "flamegate-ext") {
		t.Fatalf("WASM.StoreIndexURL = %q, want flamegate-ext index", cfg.WASM.StoreIndexURL)
	}
	if cfg.WASM.GithubTokenEnv != "GITHUB_TOKEN" {
		t.Fatalf("WASM.GithubTokenEnv = %q, want GITHUB_TOKEN", cfg.WASM.GithubTokenEnv)
	}
	if cfg.WASM.StoreCacheTTL <= 0 {
		t.Fatalf("WASM.StoreCacheTTL = %v, want >0 (default 10m)", cfg.WASM.StoreCacheTTL)
	}
}

func TestWASMConfigEnvOverrideExtPublicKeys(t *testing.T) {
	unsetEnv(t, "FLAMEGATE_WASM__EXT_PUBLIC_KEYS")
	t.Setenv("FLAMEGATE_WASM__EXT_PUBLIC_KEYS", "abc123,  def456  ,ghi789")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(cfg.WASM.ExtPublicKeys) != 3 {
		t.Fatalf("WASM.ExtPublicKeys len = %d, want 3; got %v", len(cfg.WASM.ExtPublicKeys), cfg.WASM.ExtPublicKeys)
	}
	if cfg.WASM.ExtPublicKeys[1] != "def456" {
		t.Fatalf("WASM.ExtPublicKeys[1] = %q, want def456", cfg.WASM.ExtPublicKeys[1])
	}
}

func TestWASMConfigTOML(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "flamegate.toml")
	tomlContent := `[wasm]
ext_dir = "/opt/extensions"
max_memory_mb = 64
max_inst = 4
default_timeout = "30s"
hot_reload_interval = "20s"
allowed_hosts = ["api.openai.com", "api.anthropic.com"]
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("write toml: %v", err)
	}

	cfg, err := Load(tomlPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.WASM.ExtDir != "/opt/extensions" {
		t.Fatalf("WASM.ExtDir = %q, want /opt/extensions", cfg.WASM.ExtDir)
	}
	if cfg.WASM.MaxMemoryMB != 64 {
		t.Fatalf("WASM.MaxMemoryMB = %d, want 64", cfg.WASM.MaxMemoryMB)
	}
	if cfg.WASM.MaxInst != 4 {
		t.Fatalf("WASM.MaxInst = %d, want 4", cfg.WASM.MaxInst)
	}
	if cfg.WASM.DefaultTimeout != 30*time.Second {
		t.Fatalf("WASM.DefaultTimeout = %v, want 30s", cfg.WASM.DefaultTimeout)
	}
	if cfg.WASM.HotReloadInterval != 20*time.Second {
		t.Fatalf("WASM.HotReloadInterval = %v, want 20s", cfg.WASM.HotReloadInterval)
	}
	if len(cfg.WASM.AllowedHosts) != 2 {
		t.Fatalf("WASM.AllowedHosts len = %d, want 2", len(cfg.WASM.AllowedHosts))
	}
	if cfg.WASM.AllowedHosts[0] != "api.openai.com" || cfg.WASM.AllowedHosts[1] != "api.anthropic.com" {
		t.Fatalf("WASM.AllowedHosts = %v, want [api.openai.com api.anthropic.com]", cfg.WASM.AllowedHosts)
	}
}
