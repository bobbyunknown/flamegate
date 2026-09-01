package schema

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestAllModelsIncludesExtensions verifies that AllModels includes the
// Extension and ExtensionModel structs, so db.Migrate() auto-creates
// both tables.
func TestAllModelsIncludesExtensions(t *testing.T) {
	found := map[string]bool{}
	for _, m := range AllModels() {
		switch m.(type) {
		case *Extension:
			found["Extension"] = true
		case *ExtensionModel:
			found["ExtensionModel"] = true
		}
	}
	if !found["Extension"] {
		t.Fatalf("AllModels() missing *Extension")
	}
	if !found["ExtensionModel"] {
		t.Fatalf("AllModels() missing *ExtensionModel")
	}
}

// TestExtensionTablesMigrate verifies that AutoMigrate creates the
// extensions and extension_models tables with the expected columns.
func TestExtensionTablesMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(AllModels()...); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// Verify tables exist by querying them.
	if !db.Migrator().HasTable("extensions") {
		t.Fatalf("extensions table not created")
	}
	if !db.Migrator().HasTable("extension_models") {
		t.Fatalf("extension_models table not created")
	}

	// Verify key columns exist.
	for _, col := range []string{"id", "slug", "state", "wasm_path", "capabilities", "entrypoints"} {
		if !db.Migrator().HasColumn(&Extension{}, col) {
			t.Fatalf("extensions table missing column %q", col)
		}
	}
	for _, col := range []string{"id", "extension_id", "source", "slug"} {
		if !db.Migrator().HasColumn(&ExtensionModel{}, col) {
			t.Fatalf("extension_models table missing column %q", col)
		}
	}
}
