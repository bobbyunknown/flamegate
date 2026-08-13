package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/sirupsen/logrus"
)

// PanicRecovery recovers from panics and logs the stack trace.
func PanicRecovery() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logrus.WithFields(logrus.Fields{
						"error":  rec,
						"stack":  string(debug.Stack()),
						"method": r.Method,
						"path":   r.URL.Path,
					}).Error("panic recovered")
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
