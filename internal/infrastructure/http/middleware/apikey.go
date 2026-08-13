package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/handlers"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/identity"
	"github.com/bobbyunknown/flamegate/internal/shared/consolelog"
)

// APIKeyAuth authenticates requests via Bearer token or x-api-key header.
// Sets the authenticated key in context via handlers.AuthedKeyCtxKey.
func APIKeyAuth(idSvc *identity.Service, log *logrus.Logger, conLog *consolelog.Buffer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractToken(r)
			if token == "" {
				writeErr(w, http.StatusUnauthorized, "missing API key")
				return
			}

			key, err := idSvc.Authenticate(r.Context(), token)
			if err != nil {
				if errors.Is(err, identity.ErrUnauthorized) {
					conLog.Log("WARN", "Rejected request · invalid API key",
						fmt.Sprintf("%s %s", r.Method, r.URL.Path))
					writeErr(w, http.StatusUnauthorized, "invalid API key")
					return
				}
				log.Error("auth lookup failed", "err", err)
				conLog.Log("ERROR", "Authentication lookup failed", err.Error())
				writeErr(w, http.StatusInternalServerError, "authentication error")
				return
			}
			conLog.Log("DEBUG", fmt.Sprintf("Authenticated key %q", key.Name),
				fmt.Sprintf("Key:     %s (%s)\nRequest: %s %s", key.Name, key.ID, r.Method, r.URL.Path))

			ctx := context.WithValue(r.Context(), handlers.AuthedKeyCtxKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
