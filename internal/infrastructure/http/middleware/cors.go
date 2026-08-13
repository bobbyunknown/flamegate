package middleware

import (
	"net"
	"net/http"

	"github.com/bobbyunknown/flamegate/internal/config"
)

// LoopbackOnly rejects non-loopback clients when bind-loopback-only is set.
// Guards the dashboard/admin surface in local single-user mode.
func LoopbackOnly(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Security.BindLoopbackOnly {
				next.ServeHTTP(w, r)
				return
			}
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				writeErr(w, http.StatusForbidden, "dashboard is restricted to loopback access")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
