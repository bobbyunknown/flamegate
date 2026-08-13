package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// collapseDoubleV1 rewrites /v1/v1 → /v1 before routing.
// The Anthropic SDK (Claude Code) always appends /v1/messages to
// ANTHROPIC_BASE_URL, so a base URL ending in /v1 yields /v1/v1/messages.
func CollapseDoubleV1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/v1/v1/"
		if len(r.URL.Path) > len(prefix) && r.URL.Path[:len(prefix)] == prefix {
			r.URL.Path = "/v1/" + r.URL.Path[len(prefix):]
		} else if r.URL.Path == "/v1/v1" {
			r.URL.Path = "/v1"
		}
		next.ServeHTTP(w, r)
	})
}

// --- IP limiter (internal to package) ---

type ipLimiter struct {
	rate     int
	window   time.Duration
	mu       sync.Mutex
	visitors map[string]*visitor
	stop     chan struct{}
}

type visitor struct {
	count    int
	lastSeen time.Time
}

func newIPLimiter(rate int, window time.Duration) *ipLimiter {
	l := &ipLimiter{
		rate:     rate,
		window:   window,
		visitors: make(map[string]*visitor),
		stop:     make(chan struct{}),
	}
	go l.cleanup()
	return l
}

func (l *ipLimiter) Allow(ip string) bool {
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

func (l *ipLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-ticker.C:
			l.mu.Lock()
			for ip, v := range l.visitors {
				if time.Since(v.lastSeen) > l.window {
					delete(l.visitors, ip)
				}
			}
			l.mu.Unlock()
		}
	}
}

func extractIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func extractToken(r *http.Request) string {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
			return strings.TrimSpace(after)
		}
		return strings.TrimSpace(authHeader)
	}
	if k := r.Header.Get("x-api-key"); k != "" {
		return strings.TrimSpace(k)
	}
	return ""
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`)) //nolint:errcheck // best-effort write
}
