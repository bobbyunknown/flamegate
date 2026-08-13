package ports

import (
	"context"
	"time"
)

// Clock abstracts time.Now() for deterministic testing.
type Clock interface {
	Now() time.Time
}

// CacheStore is the contract for the caching layer.
type CacheStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
