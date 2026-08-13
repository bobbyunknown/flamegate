package proxy

import (
	"context"

	base "github.com/bobbyunknown/flamegate/internal/infrastructure/http/handlers"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func authedKey(ctx context.Context) (schema.APIKey, bool) {
	return base.AuthedKey(ctx)
}
