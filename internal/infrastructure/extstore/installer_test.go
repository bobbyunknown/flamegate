package extstore

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
)

// fakeConfig satisfies hostConfig and optionally the index URLer.
type fakeConfig struct {
	extDir        string
	storeIndexURL string
}

func (f *fakeConfig) ExtDir() string { return f.extDir }

func (f *fakeConfig) StoreIndexURL() string {
	if f.storeIndexURL != "" {
		return f.storeIndexURL
	}
	return "https://raw.githubusercontent.com/bobbyunknown/flamegate-ext/main/store/index.json"
}

// fakeCompiler records compiled slugs.
type fakeCompiler struct{ compiled []string }

func (f *fakeCompiler) Compile(_ context.Context, slug string, _ []byte, _ wasm.ExtensionConfig) error {
	f.compiled = append(f.compiled, slug)
	return nil
}

// fakeRepo records created extensions.
type fakeRepo struct{ created []schema.Extension }

func (f *fakeRepo) FindBySlug(_ context.Context, _ string) (schema.Extension, error) {
	return schema.Extension{}, os.ErrNotExist
}

func (f *fakeRepo) Create(_ context.Context, e schema.Extension) error {
	f.created = append(f.created, e)
	return nil
}

func (f *fakeRepo) Update(_ context.Context, e schema.Extension) error {
	f.created = append(f.created, e)
	return nil
}

// buildSignedExtZip builds a zip with schema.json + demo.wasm + SHA256SUMS,
// optionally signed by priv. Returns the zip path and (if signed) pubkey hex.
func buildSignedExtZip(t *testing.T, priv ed25519.PrivateKey, sign bool) (string, string) {
	t.Helper()
	files := map[string]string{
		"schema.json": `{"slug":"demo","name":"Demo","version":"1.0.0","entrypoints":{"chat":"invoke"}}`,
		"demo.wasm":   "fake-wasm-bytes",
	}
	var lines string
	lines += sha256sum(t, files["schema.json"]) + "  schema.json\n"
	lines += sha256sum(t, files["demo.wasm"]) + "  demo.wasm\n"
	files["SHA256SUMS"] = lines
	var pubHex string
	if sign {
		sig := ed25519.Sign(priv, []byte(files["SHA256SUMS"]))
		files["SHA256SUMS.sig"] = string(sig)
		pubHex = hex.EncodeToString(priv.Public().(ed25519.PublicKey))
	}
	return buildTestZip(t, files), pubHex
}

func TestInstallerRemoteURL(t *testing.T) {
	zipPath, _ := buildSignedExtZip(t, nil, false)
	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	url := serveBytes(t, "demo.zip", zipBytes)

	extDir := t.TempDir()
	httpc := &http.Client{}
	repo := &fakeRepo{}

	inst := NewInstaller(
		&fakeConfig{extDir: extDir},
		NewGithubClient(httpc, nil, NewTTLCache(), time.Minute),
		NewIndexStore(httpc, NewTTLCache()),
		NewDownloader(httpc),
		&fakeCompiler{},
		repo,
		nil,  // no pubkeys → unsigned community
		true, // allowUnsigned
		nil,  // extConf nil → skip engine compile in unit test
	)

	res, err := inst.Install(context.Background(), "url:"+url)
	if err != nil {
		t.Fatal(err)
	}
	if res.Slug != "demo" {
		t.Fatalf("slug = %q, want demo", res.Slug)
	}
	if res.Trust != TrustCommunity {
		t.Fatalf("trust = %v, want community", res.Trust)
	}
	if _, err := os.Stat(extDir + "/demo/demo.wasm"); err != nil {
		t.Fatalf("wasm not placed in ExtDir: %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created rows = %d, want 1", len(repo.created))
	}
	if repo.created[0].SourceURI != "url:"+url {
		t.Fatalf("SourceURI = %q", repo.created[0].SourceURI)
	}
	if repo.created[0].TrustLevel != string(TrustCommunity) {
		t.Fatalf("TrustLevel = %q", repo.created[0].TrustLevel)
	}
}

func TestInstallerStoreOfficialSigned(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	zipPath, _ := buildSignedExtZip(t, priv, true)
	zipBytes, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	assetURL := serveBytes(t, "demo-1.0.0.zip", zipBytes)

	// Serve a fake GitHub API + store index from one httptest server.
	srv := httptestServer(t, map[string]string{
		"/index.json": `{"version":1,"extensions":[{"slug":"demo","source":{"type":"github","owner":"acme","repo":"ext","tag_prefix":"demo-v","asset_pattern":"demo-{version}.zip"}}]}`,
	})
	ghURL := srv2WithRelease(t, "demo-v1.0.0", assetURL)
	defer ghURL.Close()

	// Two servers: index at srv, releases at ghURL.
	extDir := t.TempDir()

	gh := NewGithubClient(ghURL.Client(), nil, NewTTLCache(), time.Minute)
	gh.baseURL = ghURL.URL

	inst := NewInstaller(
		&fakeConfig{extDir: extDir, storeIndexURL: srv.URL + "/index.json"},
		gh,
		NewIndexStore(srv.Client(), NewTTLCache()),
		NewDownloader(ghURL.Client()),
		&fakeCompiler{},
		&fakeRepo{},
		[]string{hex.EncodeToString(pub)},
		true,
		nil,
	)

	res, err := inst.Install(context.Background(), "store:demo")
	if err != nil {
		t.Fatal(err)
	}
	if res.Trust != TrustOfficial {
		t.Fatalf("trust = %v, want official", res.Trust)
	}
	if res.InstalledRef != "demo-v1.0.0" {
		t.Fatalf("InstalledRef = %q", res.InstalledRef)
	}
}

// srv2WithRelease serves a fake GitHub releases endpoint on its own server.
func srv2WithRelease(t *testing.T, tag, assetURL string) *httptest.Server {
	t.Helper()
	body := fmt.Sprintf(`[{"tag_name":%q,"published_at":"2026-08-01T00:00:00Z","assets":[{"name":"demo-1.0.0.zip","browser_download_url":%q,"size":10}]}]`, tag, assetURL)
	return httptestServer(t, map[string]string{
		"/repos/acme/ext/releases": body,
	})
}

func TestInstallerRejectsBadSource(t *testing.T) {
	inst := NewInstaller(
		&fakeConfig{extDir: t.TempDir()},
		nil, nil, nil, nil, nil,
		nil, true,
		nil,
	)
	_, err := inst.Install(context.Background(), "github:")
	if err == nil || !strings.Contains(err.Error(), "invalid source") {
		t.Fatalf("err = %v", err)
	}
}