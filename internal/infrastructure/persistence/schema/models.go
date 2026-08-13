// Package schema defines GORM models for Atlas schema generation.
// These models map 1:1 with the store.Models but add GORM annotations
// (table names, column types, indexes) for schema-as-code migration.
//
// The schema models are used ONLY by:
//   - db/gormschema/main.go (Atlas provider)
//   - persistence/gorm.go (GORM connection with AutoMigrate disabled)
//
// Domain entities live in internal/domain/ and do NOT import this package.
package schema

// AllModels returns all schema models for Atlas provider registration.
func AllModels() []interface{} {
	return []interface{}{
		&Tenant{},
		&APIKey{},
		&APIKeyModelAccess{},
		&Plan{},
		&Account{},
		&Chain{},
		&ChainStep{},
		&UsageRecord{},
		&Budget{},
		&AuditEntry{},
		&ModelCooldown{},
		&ChainRotation{},
		&TargetRotation{},
		&AccountAffinity{},
		&AccountHealth{},
		&ResourceSample{},
		&GuardrailPolicy{},
		&GuardrailLog{},
		&ResourceBucket{},
		&ModelAlias{},
		&ProxyPool{},
		&Settings{},
		&CustomProvider{},
		&CustomModel{},
		&Extension{},
		&ExtensionModel{},
		&User{},
	}
}
