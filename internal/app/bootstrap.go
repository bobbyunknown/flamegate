package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/identity"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// Bootstrap creates an initial API key for local use and returns its plaintext.
// It is invoked by `flamegate -bootstrap` so a fresh install is immediately
// usable without a dashboard. The plaintext is shown once and never recoverable.
func Bootstrap(ctx context.Context, cfg config.Config, name string) (string, error) {
	dataDir, err := ResolveDataDir(cfg)
	if err != nil {
		return "", err
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
		return "", err
	}
	defer db.Close() //nolint:errcheck // best-effort close

	if err := db.Migrate(); err != nil {
		return "", fmt.Errorf("bootstrap: migrate: %w", err)
	}
	if err := db.EnsureDefault(); err != nil {
		return "", fmt.Errorf("bootstrap: ensure tenant: %w", err)
	}

	if name == "" {
		name = "default"
	}
	issued, err := identity.New(db.APIKeys()).Create(ctx, schema.DefaultTenantID, "", name)
	if err != nil {
		return "", fmt.Errorf("bootstrap: create key: %w", err)
	}
	_ = db.APIKeys().SetPlanID(ctx, issued.Record.ID, "default")
	return issued.Plaintext, nil
}
