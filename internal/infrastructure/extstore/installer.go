package extstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/wasm"
)

// ExtensionRepo is the persistence surface the installer needs. Implemented by
// *persistence.ExtensionRepo (see db.Extensions()).
type ExtensionRepo interface {
	FindBySlug(ctx context.Context, slug string) (schema.Extension, error)
	Create(ctx context.Context, e schema.Extension) error
}

// Compiler compiles WASM bytes for a slug. Implemented by *wasm.Engine.
type Compiler interface {
	Compile(ctx context.Context, slug string, wasmBytes []byte, cfg wasm.ExtensionConfig) error
}

// hostConfig supplies the repository/wasm knobs used by the pipeline.
type hostConfig interface {
	ExtDir() string
}

// InstallResult reports what was installed.
type InstallResult struct {
	Slug         string
	Version      string
	SourceURI    string
	InstalledRef string
	Checksum     string
	Trust        TrustLevel
}

// StoreItem is a catalog entry surfaced by the dashboard store listing.
type StoreItem struct {
	Slug        string       `json:"slug"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Version     string       `json:"version,omitempty"`
	Checksum    string       `json:"checksum,omitempty"`
}

// ListStore returns all catalog entries with their latest resolved version and
// runtime checksum (best-effort; version/checksum empty on resolution failure).
func (i *Installer) ListStore(ctx context.Context) ([]StoreItem, error) {
	idx, err := i.index.Fetch(ctx, i.indexURL())
	if err != nil {
		return nil, err
	}
	items := make([]StoreItem, 0, len(idx.Extensions))
	for _, e := range idx.Extensions {
		item := StoreItem{Slug: e.Slug, Name: e.Name, Description: e.Description}
		if rel, err := i.gh.LatestRelease(ctx, e.Source.Owner, e.Source.Repo, e.Source.TagPrefix, e.Source.AssetPattern); err == nil && len(rel.Assets) > 0 {
			item.Version = rel.Tag
		}
		items = append(items, item)
	}
	return items, nil
}

// GetStore returns a single catalog entry with live version info.
func (i *Installer) GetStore(ctx context.Context, slug string) (*StoreItem, error) {
	idx, err := i.index.Fetch(ctx, i.indexURL())
	if err != nil {
		return nil, err
	}
	e, err := idx.Find(slug)
	if err != nil {
		return nil, err
	}
	item := StoreItem{Slug: e.Slug, Name: e.Name, Description: e.Description}
	if rel, rerr := i.gh.LatestRelease(ctx, e.Source.Owner, e.Source.Repo, e.Source.TagPrefix, e.Source.AssetPattern); rerr == nil && len(rel.Assets) > 0 {
		item.Version = rel.Tag
	}
	return &item, nil
}

// Installer runs the unified install pipeline shared by CLI and admin API.
type Installer struct {
	cfg           hostConfig
	gh            *GithubClient
	index         *IndexStore
	dl            *Downloader
	engine        Compiler
	repo          ExtensionRepo
	extConf       func(schema.Extension, map[string]string) wasm.ExtensionConfig
	pubKeys       []string
	allowUnsigned bool
	isOfficial    func(spec SourceSpec) bool
}

// NewInstaller wires the pipeline. extConf builds the engine extension config
// (wasm.ExtensionConfig) from a parsed schema; engine may be nil in unit tests
// that only exercise resolution/verification. isOfficial reports whether a
// source is the sealed "official" tier (store source).
func NewInstaller(
	cfg hostConfig,
	gh *GithubClient,
	index *IndexStore,
	dl *Downloader,
	engine Compiler,
	repo ExtensionRepo,
	pubKeys []string,
	allowUnsigned bool,
	extConf func(schema.Extension, map[string]string) wasm.ExtensionConfig,
) *Installer {
	return &Installer{
		cfg: cfg, gh: gh, index: index, dl: dl, engine: engine, repo: repo,
		extConf:       extConf,
		pubKeys:       pubKeys,
		allowUnsigned: allowUnsigned,
		isOfficial:    func(spec SourceSpec) bool { return spec.Kind == SourceStore },
	}
}

// Install resolves, downloads, verifies, unpacks, compiles, and records slug.
func (i *Installer) Install(ctx context.Context, input string) (*InstallResult, error) {
	spec, err := ParseSource(input)
	if err != nil {
		return nil, err
	}

	// Resolve to a concrete asset URL (or local dir).
	var assetURL string
	var installedRef, version string
	switch spec.Kind {
	case SourceURL:
		assetURL = spec.URL
	case SourceGitHub:
		rel, err := i.gh.LatestRelease(ctx, spec.Owner, spec.Repo, spec.Ref, extAssetPattern(spec.Ref))
		if err != nil {
			return nil, err
		}
		if len(rel.Assets) == 0 {
			return nil, ErrNoReleaseAsset
		}
		assetURL = rel.Assets[0].URL
		installedRef = rel.Tag
		version = rel.Tag
	case SourceStore:
		idx, err := i.index.Fetch(ctx, i.indexURL())
		if err != nil {
			return nil, err
		}
		entry, err := idx.Find(spec.Slug)
		if err != nil {
			return nil, err
		}
		if entry.Source.Type != "github" {
			return nil, fmt.Errorf("unsupported store source type %q", entry.Source.Type)
		}
		rel, err := i.gh.LatestRelease(ctx, entry.Source.Owner, entry.Source.Repo, entry.Source.TagPrefix, entry.Source.AssetPattern)
		if err != nil {
			return nil, err
		}
		if len(rel.Assets) == 0 {
			return nil, ErrNoReleaseAsset
		}
		assetURL = rel.Assets[0].URL
		installedRef = rel.Tag
		version = rel.Tag
	case SourceLocal:
		_, err := os.Stat(filepath.Join(spec.LocalDir, "schema.json"))
		if err != nil {
			return nil, fmt.Errorf("install: local %s missing schema.json: %w", spec.LocalDir, err)
		}
		return i.installLocal(ctx, spec)
	default:
		return nil, fmt.Errorf("%w: %q", ErrBadSource, spec.Kind)
	}

	return i.installRemote(ctx, spec, assetURL, version, installedRef)
}

func (i *Installer) installLocal(ctx context.Context, spec SourceSpec) (*InstallResult, error) {
	slug, err := slugFromDir(spec.LocalDir)
	if err != nil {
		return nil, err
	}
	target := filepath.Join(i.cfg.ExtDir(), slug)
	// Keep the existing local install path: copy schema.json + wasm.
	if err := copyLocalExt(spec.LocalDir, target); err != nil {
		return nil, err
	}
	ext := schema.Extension{
		ID: slug, Slug: slug, Name: slug, Version: "local",
		State: "ACTIVE", SourceURI: "", TrustLevel: string(TrustLocal), InstalledAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := i.repo.Create(ctx, ext); err != nil {
		return nil, fmt.Errorf("install: save: %w", err)
	}
	return &InstallResult{Slug: slug, Version: "local", Trust: TrustLocal}, nil
}

func (i *Installer) installRemote(ctx context.Context, spec SourceSpec, assetURL, version, installedRef string) (*InstallResult, error) {
	zipPath, err := i.dl.FetchToTemp(ctx, assetURL, 32<<20)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(zipPath) }()

	checksum, err := hashFile(zipPath)
	if err != nil {
		return nil, err
	}

	// Tiered verification.
	ver, err := VerifyArchive(zipPath, i.pubKeys, i.allowUnsigned, i.isOfficial(spec))
	if err != nil {
		return nil, err
	}

	// Unpack to a system temp dir (never inside ExtDir), then validate schema.
	stageDir, err := os.MkdirTemp("", "extstage-*")
	if err != nil {
		return nil, fmt.Errorf("install: mkdir temp: %w", err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()
	if err := UnpackArchive(ctx, zipPath, stageDir); err != nil {
		return nil, err
	}

	slug, _, entrypoints, err := readSchema(stageDir)
	if err != nil {
		return nil, err
	}
	wasmPath, err := findWasm(stageDir, slug)
	if err != nil {
		return nil, err
	}
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, err
	}
	stub := schema.Extension{Slug: slug}
	if i.extConf != nil {
		if i.engine == nil {
			return nil, fmt.Errorf("install: wasm engine unavailable")
		}
		if err := i.engine.Compile(ctx, slug, wasmBytes, i.extConf(stub, entrypoints)); err != nil {
			return nil, fmt.Errorf("install: compile: %w", err)
		}
	}

	// Atomic move into ExtDir.
	target := filepath.Join(i.cfg.ExtDir(), slug)
	if err := atomicReplaceDir(stageDir, target); err != nil {
		return nil, err
	}

	ext := schema.Extension{
		ID: slug, Slug: slug, Name: slug, Version: version,
		State: "ACTIVE", SourceURI: sourceURI(spec, installedRef),
		Checksum: checksum, InstalledRef: installedRef, TrustLevel: string(ver.Trust),
		InstalledAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := i.repo.Create(ctx, ext); err != nil {
		return nil, fmt.Errorf("install: save: %w", err)
	}
	return &InstallResult{Slug: slug, Version: version, SourceURI: ext.SourceURI, InstalledRef: installedRef, Checksum: checksum, Trust: ver.Trust}, nil
}

// --- small helpers ---------------------------------------------------------

func (i *Installer) indexURL() string {
	if s, ok := i.cfg.(interface{ StoreIndexURL() string }); ok {
		return s.StoreIndexURL()
	}
	return "https://raw.githubusercontent.com/bobbyunknown/flamegate-ext/main/store/index.json"
}

func extAssetPattern(ref string) string {
	if ref == "" {
		return "{version}.zip"
	}
	return ref + ".zip"
}

func sourceURI(spec SourceSpec, ref string) string {
	switch spec.Kind {
	case SourceStore:
		return "store:" + spec.Slug
	case SourceGitHub:
		return "github:" + spec.Owner + "/" + spec.Repo + "@" + ref
	case SourceURL:
		return "url:" + spec.URL
	default:
		return ""
	}
}

func slugFromDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.Base(abs), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// readSchema finds schema.json in a dir and extracts slug + entrypoints.
func readSchema(dir string) (string, []byte, map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "schema.json"))
	if err != nil {
		return "", nil, nil, fmt.Errorf("install: missing schema.json: %w", err)
	}
	var s struct {
		Slug       string            `json:"slug"`
		Entrypoints map[string]string `json:"entrypoints"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return "", nil, nil, fmt.Errorf("install: parse schema.json: %w", err)
	}
	if s.Slug == "" {
		return "", nil, nil, fmt.Errorf("install: schema.json missing slug")
	}
	return s.Slug, data, s.Entrypoints, nil
}

func findWasm(dir, slug string) (string, error) {
	// prefer <slug>.wasm, then any .wasm
	candidates := []string{filepath.Join(dir, slug+".wasm")}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".wasm" {
			candidates = append(candidates, filepath.Join(dir, e.Name()))
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("install: no .wasm found in archive")
}

func copyLocalExt(srcDir, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"schema.json"} {
		src := filepath.Join(srcDir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("install: %w", err)
		}
		if err := os.WriteFile(filepath.Join(target, name), data, 0o644); err != nil {
			return err
		}
	}
	wasm, err := findWasm(srcDir, filepath.Base(srcDir))
	if err != nil {
		return err
	}
	data, err := os.ReadFile(wasm)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(target, filepath.Base(wasm)), data, 0o644)
}

func atomicReplaceDir(src, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.RemoveAll(target); err != nil {
			return fmt.Errorf("install: replace existing: %w", err)
		}
	}
	return os.Rename(src, target)
}