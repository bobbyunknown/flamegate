// Package proxy resolves proxy pool bindings into concrete proxy configuration
// that connectors can apply to outbound HTTP requests.
package proxy

import (
	"context"
	"fmt"

	core "github.com/bobbyunknown/flamegate/internal/domain"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// PoolSource resolves a proxy pool by id.
type PoolSource interface {
	Get(ctx context.Context, id string) (schema.ProxyPool, error)
}

// ResolvePool looks up a proxy pool by ID and injects proxy config into creds.
// If poolID is empty or "__none__", creds are returned unchanged.
func ResolvePool(ctx context.Context, pools PoolSource, poolID string, creds *core.Credentials) error {
	if poolID == "" || poolID == "__none__" {
		return nil
	}
	pool, err := pools.Get(ctx, poolID)
	if err != nil {
		return fmt.Errorf("proxy: resolve pool %q: %w", poolID, err)
	}
	if !pool.IsActive {
		return nil // pool disabled, use direct connection
	}

	switch pool.Type {
	case "vercel", "cloudflare", "deno":
		creds.RelayURL = pool.ProxyURL
	default: // "http" or any other type
		creds.ProxyURL = pool.ProxyURL
	}
	creds.NoProxy = pool.NoProxy
	creds.StrictProxy = pool.Strict
	return nil
}
