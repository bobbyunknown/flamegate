package middleware

import (
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/sirupsen/logrus"
)

// RequestLogging logs each request with method, path, status, and latency.
// At debug level it also logs origin, remote addr, and user-agent.
func RequestLogging(logger *logrus.Logger) func(http.Handler) http.Handler {
	debug := logger.GetLevel() >= logrus.DebugLevel
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			fields := logrus.Fields{
				"method":  r.Method,
				"path":    r.URL.Path,
				"status":  ww.Status(),
				"latency": time.Since(start).String(),
			}
			if debug {
				if origin := r.Header.Get("Origin"); origin != "" {
					fields["origin"] = origin
				}
				fields["remote"] = r.RemoteAddr
				if ua := r.UserAgent(); ua != "" {
					fields["ua"] = ua
				}
				fields["bytes"] = ww.BytesWritten()
			}
			logger.WithFields(fields).Info("request")
		})
	}
}
