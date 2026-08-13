package middleware

import (
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
)

const sessionCookie = "fg_session"

// SessionAuth protects the admin API with a valid dashboard session —
// either from a cookie or an Authorization: Bearer JWT header.
func SessionAuth(authSvc *auth.Service, log *logrus.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				token = sessionToken(r)
			}
			if token == "" {
				writeErr(w, http.StatusUnauthorized, "session required")
				return
			}
			if !authSvc.VerifySession(token) {
				writeErr(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken extracts a JWT from the Authorization header. Returns "" if absent.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

func sessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}
