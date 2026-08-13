package handlers

import (
	"context"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// ctxKey is an unexported context key type to avoid collisions.
type ctxKey int

const apiKeyCtxKey ctxKey = iota

// AuthedKey returns the authenticated API key from context.
// This is set by the middleware.APIKeyAuth middleware.
func AuthedKey(ctx context.Context) (schema.APIKey, bool) {
	k, ok := ctx.Value(apiKeyCtxKey).(schema.APIKey)
	return k, ok
}

// TenantOf returns the tenant id for an authenticated key.
func TenantOf(key schema.APIKey) string {
	if key.TenantID != "" {
		return key.TenantID
	}
	return schema.DefaultTenantID
}

// AuthedKeyCtxKey is exported so middleware can use the same context key.
var AuthedKeyCtxKey = apiKeyCtxKey
