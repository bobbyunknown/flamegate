package wasm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/connectors"
)

var (
	// ErrInvalidSchema indicates a schema.json file is missing required fields.
	ErrInvalidSchema = errors.New("wasm: invalid extension schema")
	// ErrSlugCollision indicates the extension slug matches a native provider ID.
	ErrSlugCollision = errors.New("wasm: slug collides with native provider")
	// ErrSlugFormat indicates the slug doesn't match the allowed format.
	ErrSlugFormat = errors.New("wasm: invalid slug format")
)

// slugRegex validates extension slugs: lowercase alphanumeric, hyphens, underscores.
var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}[a-z0-9]$`)

// ExtensionSchema represents the parsed schema.json of a WASM extension.
type ExtensionSchema struct {
	Slug              string            `json:"slug"`
	Name              string            `json:"name"`
	Version           string            `json:"version"`
	Description       string            `json:"description,omitempty"`
	MaxInstances      int               `json:"max_instances,omitempty"`
	Entrypoints       map[string]string `json:"entrypoints"`
	Timeout           int               `json:"timeout,omitempty"`
	DefaultAccountKey string            `json:"default_account_key,omitempty"`
}

// ExtensionManifest holds the paths and parsed schema for a discovered extension.
type ExtensionManifest struct {
	Slug      string
	Dir       string
	WasmPath  string
	Schema    ExtensionSchema
	WasmBytes []byte
}

// Scan discovers extensions in the given directory. Each extension is a directory
// containing schema.json and a .wasm file named <slug>.wasm.
// Returns ErrSlugCollision if any extension slug matches a native provider ID.
func Scan(dir string) ([]ExtensionManifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("wasm: scan dir %s: %w", dir, err)
	}

	// Build native slug set from runtime catalog.
	nativeSlugs := make(map[string]bool)
	for _, p := range connectors.Catalog() {
		nativeSlugs[strings.ToLower(p.ID)] = true
	}

	var manifests []ExtensionManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		m, err := scanOne(dir, entry.Name(), nativeSlugs)
		if err != nil {
			return nil, err
		}
		if m != nil {
			manifests = append(manifests, *m)
		}
	}

	return manifests, nil
}

// scanOne validates a single extension directory and returns its manifest.
// Returns nil manifest for non-extension directories (no schema.json).
func scanOne(baseDir, slug string, nativeSlugs map[string]bool) (*ExtensionManifest, error) {
	// Validate slug format.
	if !IsValidSlug(slug) {
		return nil, fmt.Errorf("%w: %q must be lowercase alphanumeric with hyphens/underscores (1-64 chars)", ErrSlugFormat, slug)
	}

	// Check native slug collision.
	if nativeSlugs[slug] {
		return nil, fmt.Errorf("%w: %q is a built-in provider", ErrSlugCollision, slug)
	}

	extDir := filepath.Join(baseDir, slug)

	// Check for schema.json.
	schemaPath := filepath.Join(extDir, "schema.json")
	schemaData, err := os.ReadFile(schemaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // not an extension directory, skip
		}
		return nil, fmt.Errorf("wasm: read schema %s: %w", schemaPath, err)
	}

	var schema ExtensionSchema
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return nil, fmt.Errorf("%w: %s: invalid json: %s", ErrInvalidSchema, slug, err)
	}

	// Validate required fields.
	if err := validateSchema(slug, &schema); err != nil {
		return nil, err
	}

	// Find .wasm file: try <slug>.wasm first, then any .wasm in directory.
	wasmPath := filepath.Join(extDir, slug+".wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		// Try any .wasm file.
		wasmPath, err = findWasmFile(extDir)
		if err != nil {
			return nil, fmt.Errorf("wasm: %s: %w", slug, err)
		}
	}

	// Read wasm bytes.
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("wasm: %s: read wasm %s: %w", slug, wasmPath, err)
	}

	return &ExtensionManifest{
		Slug:      slug,
		Dir:       extDir,
		WasmPath:  wasmPath,
		WasmBytes: wasmBytes,
		Schema:    schema,
	}, nil
}

// validateSchema checks that all required fields are present and valid.
func validateSchema(dirSlug string, s *ExtensionSchema) error {
	if s.Slug == "" {
		return fmt.Errorf("%w: missing 'slug'", ErrInvalidSchema)
	}
	if s.Slug != dirSlug {
		return fmt.Errorf("%w: schema slug %q doesn't match directory name %q", ErrInvalidSchema, s.Slug, dirSlug)
	}
	if s.Name == "" {
		return fmt.Errorf("%w: missing 'name'", ErrInvalidSchema)
	}
	if s.Version == "" {
		return fmt.Errorf("%w: missing 'version'", ErrInvalidSchema)
	}
	if len(s.Entrypoints) == 0 {
		return fmt.Errorf("%w: missing 'entrypoints'", ErrInvalidSchema)
	}
	if _, ok := s.Entrypoints["chat"]; !ok {
		return fmt.Errorf("%w: entrypoints must include 'chat'", ErrInvalidSchema)
	}
	return nil
}

// IsValidSlug checks slug format: lowercase alphanumeric, hyphens, underscores.
func IsValidSlug(slug string) bool {
	if len(slug) == 0 || len(slug) > 64 {
		return false
	}
	// Single char must be alphanumeric.
	if len(slug) == 1 {
		return slug[0] >= 'a' && slug[0] <= 'z' || slug[0] >= '0' && slug[0] <= '9'
	}
	return slugRegex.MatchString(slug)
}

// findWasmFile finds the first .wasm file in a directory.
func findWasmFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".wasm") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .wasm file found in %s", dir)
}
