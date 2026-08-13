package middleware

import (
	"net/http"
	"time"
)

// LoginRateLimiter allows 5 login attempts per minute per IP.
func LoginRateLimiter() func(http.Handler) http.Handler {
	limiter := newIPLimiter(5, time.Minute)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow(extractIP(r)) {
				writeErr(w, http.StatusTooManyRequests, "rate limit exceeded: too many login attempts, please try again later")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ConcurrencyLimiter caps in-flight requests.
func ConcurrencyLimiter(maxConcurrent int) func(http.Handler) http.Handler {
	sem := make(chan struct{}, maxConcurrent)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				next.ServeHTTP(w, r)
			default:
				writeErr(w, http.StatusServiceUnavailable, "server is at capacity, please retry shortly")
			}
		})
	}
}
