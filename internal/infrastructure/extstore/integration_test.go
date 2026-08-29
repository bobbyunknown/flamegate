package extstore

import (
	"context"
	"encoding/hex"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
)

// TestInstallerRealRepo verifies the full remote pipeline against a real
// in-memory SQLite schema: the installer writes files into ExtDir and persists
// a schema.Extension row with SourceURI/Checksum/InstalledRef/TrustLevel.
func TestInstallerRealRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping DB integration in -short mode")
	}

	db, err := persistence.OpenDB("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}

	zipPath, _ := buildSignedExtZip(t, nil, false)
	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	url := serveBytes(t, "demo.zip", zipBytes)
	extDir := t.TempDir()

	httpc := &http.Client{}
	inst := NewInstaller(
		&fakeConfig{extDir: extDir},
		NewGithubClient(httpc, nil, NewTTLCache(), time.Minute),
		NewIndexStore(httpc, NewTTLCache()),
		NewDownloader(httpc),
		&wasmEngineStub{},
		db.Extensions(),
		nil, true,
		func(stub schema.Extension, eps map[string]string) wasm.ExtensionConfig {
			return wasm.ExtensionConfig{Slug: stub.Slug, Entrypoints: eps}
		},
	)

	res, err := inst.Install(context.Background(), "url:"+url)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trust != TrustCommunity {
		t.Fatalf("trust = %v, want community", res.Trust)
	}
	// Files staged into ExtDir.
	if _, err := os.Stat(extDir + "/demo/demo.wasm"); err != nil {
		t.Fatalf("wasm not installed to ExtDir: %v", err)
	}
	// DB row persisted with provenance columns.
	ext, err := db.Extensions().FindBySlug(context.Background(), "demo")
	if err != nil {
		t.Fatalf("FindBySlug: %v", err)
	}
	if ext.TrustLevel != string(TrustCommunity) {
		t.Fatalf("TrustLevel = %q", ext.TrustLevel)
	}
	if ext.SourceURI != "url:"+url {
		t.Fatalf("SourceURI = %q", ext.SourceURI)
	}
	if ext.Checksum == "" {
		t.Fatal("Checksum not persisted")
	}
}

// fakeCompilerStub satisfies Compiler without a wazero runtime.
type wasmEngineStub struct{}

func (w *wasmEngineStub) Compile(_ context.Context, _ string, _ []byte, _ wasm.ExtensionConfig) error {
	return nil
}

var _ = hex.EncodeToString // keep encoding/hex import for future signature fixtures