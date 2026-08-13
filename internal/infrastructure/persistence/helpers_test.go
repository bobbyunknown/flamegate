package persistence

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bobbyunknown/flamegate/internal/application/ports"
)

// Compile-time interface assertions.
var (
	_ ports.ExtensionRepository      = (*ExtensionRepo)(nil)
	_ ports.ExtensionModelRepository = (*ExtensionModelRepo)(nil)
)

// newPersistenceTestDB opens an in-memory SQLite DB via OpenDB, runs Migrate,
// and registers t.Cleanup. Each test gets its own isolated database.
func newPersistenceTestDB(t *testing.T) *DB {
	t.Helper()
	t.Parallel()

	db, err := OpenDB("sqlite", ":memory:")
	require.NoError(t, err, "open sqlite")
	require.NoError(t, db.Migrate(), "migrate")
	t.Cleanup(func() { _ = db.Close() }) //nolint:errcheck // best-effort close

	return db
}
