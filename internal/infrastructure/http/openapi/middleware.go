package openapi

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"

	"github.com/bobbyunknown/flamegate/internal/config"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
)

// contextKey is an unexported type for context value keys in this package.
type contextKey string

// Exported context keys for use by handlers.
const (
	ActorKey        contextKey = "username"
	httpRequestKey  contextKey = "httpRequest"
	httpResponseKey contextKey = "httpResponse"
)

// InjectHTTPContext is a Huma middleware that stores the *http.Request and
// http.ResponseWriter in the Go context so typed Huma handlers can access them.
func InjectHTTPContext() func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		r, w := humachi.Unwrap(ctx)
		newCtx := context.WithValue(r.Context(), httpRequestKey, r)
		newCtx = context.WithValue(newCtx, httpResponseKey, w)
		newR := r.WithContext(newCtx)
		next(humachi.NewContext(ctx.Operation(), newR, w))
	}
}

// RequestFromContext extracts the *http.Request from a handler's context.Context.
func RequestFromContext(ctx context.Context) *http.Request {
	if r, ok := ctx.Value(httpRequestKey).(*http.Request); ok {
		return r
	}
	return nil
}

// ResponseWriterFromContext extracts the http.ResponseWriter from a handler's context.Context.
func ResponseWriterFromContext(ctx context.Context) http.ResponseWriter {
	if w, ok := ctx.Value(httpResponseKey).(http.ResponseWriter); ok {
		return w
	}
	return nil
}

// SessionAuthMiddleware verifies a JWT from the Authorization header or
// fg_session cookie. On success the username is stored in context under
// ActorKey. On failure it returns a 401 Huma error.
func SessionAuthMiddleware(authSvc *auth.Service) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		token := bearerToken(ctx)
		if token == "" {
			token = cookieToken(ctx)
		}
		if token == "" {
			writeHumaError(ctx, http.StatusUnauthorized, "session required")
			return
		}
		if !authSvc.VerifySession(token) {
			writeHumaError(ctx, http.StatusUnauthorized, "invalid or expired session")
			return
		}
		next(ctx)
	}
}

// LoopbackOnlyMiddleware rejects non-loopback clients when the config
// restricts the dashboard to loopback access.
func LoopbackOnlyMiddleware(cfg config.Config) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if !cfg.Security.BindLoopbackOnly {
			next(ctx)
			return
		}
		r, _ := humachi.Unwrap(ctx)
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			writeHumaError(ctx, http.StatusForbidden, "dashboard is restricted to loopback access")
			return
		}
		next(ctx)
	}
}

// LoginRateLimitMiddleware allows 5 login attempts per IP per minute.
func LoginRateLimitMiddleware() func(huma.Context, func(huma.Context)) {
	limiter := newIPRateLimiter(5, time.Minute)
	return func(ctx huma.Context, next func(huma.Context)) {
		r, _ := humachi.Unwrap(ctx)
		ip := extractIP(r)
		if !limiter.Allow(ip) {
			writeHumaError(ctx, http.StatusTooManyRequests, "rate limit exceeded: too many login attempts, please try again later")
			return
		}
		next(ctx)
	}
}

// writeHumaError writes a JSON error response via the Huma context body writer.
func writeHumaError(ctx huma.Context, status int, detail string) {
	ctx.SetStatus(status)
	ctx.SetHeader("Content-Type", "application/problem+json")
	_, _ = ctx.BodyWriter().Write([]byte(`{"title":"` + http.StatusText(status) + `","status":` + itoa(status) + `,"detail":"` + detail + `"}`))
}

// itoa converts an int to a string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [3]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// bearerToken extracts a JWT from the Authorization header.
func bearerToken(ctx huma.Context) string {
	h := ctx.Header("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	return ""
}

// cookieToken extracts a JWT from the fg_session cookie.
func cookieToken(ctx huma.Context) string {
	r, _ := humachi.Unwrap(ctx)
	if c, err := r.Cookie("fg_session"); err == nil && c.Value != "" {
		return c.Value
	}
	return ""
}

// extractIP returns the client IP, preferring X-Forwarded-For.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.Split(xff, ",")[0]; ip != "" {
			return strings.TrimSpace(ip)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ipRateLimiter is a simple per-IP rate limiter (5 req/min window).
type ipRateLimiter struct {
	rate     int
	window   time.Duration
	mu       sync.Mutex
	visitors map[string]*visitor
}

type visitor struct {
	count    int
	lastSeen time.Time
}

func newIPRateLimiter(rate int, window time.Duration) *ipRateLimiter {
	l := &ipRateLimiter{
		rate:     rate,
		window:   window,
		visitors: make(map[string]*visitor),
	}
	go l.cleanup()
	return l
}

func (l *ipRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, ok := l.visitors[ip]
	if !ok || time.Since(v.lastSeen) > l.window {
		l.visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
		return true
	}
	if v.count >= l.rate {
		return false
	}
	v.count++
	v.lastSeen = time.Now()
	return true
}

func (l *ipRateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		l.mu.Lock()
		for ip, v := range l.visitors {
			if time.Since(v.lastSeen) > l.window {
				delete(l.visitors, ip)
			}
		}
		l.mu.Unlock()
	}
}
