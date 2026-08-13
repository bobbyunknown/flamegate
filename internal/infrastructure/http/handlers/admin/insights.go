package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
	"github.com/bobbyunknown/flamegate/internal/shared/usagehub"
)

// sinceForPeriod maps a dashboard period query value to a lower-bound time.
// The tz parameter is an IANA timezone string (e.g. "Asia/Jakarta") sent by the
// browser so that "today" means midnight in the user's local time, not the
// server's. Falls back to the server's local time when tz is empty.
func sinceForPeriod(period, tz string) time.Time {
	loc := time.Local
	if tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)
	switch period {
	case "today":
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).UTC()
	case "24h":
		return time.Now().UTC().Add(-24 * time.Hour)
	case "week":
		return now.AddDate(0, 0, -7).UTC()
	case "month", "":
		return now.AddDate(0, -1, 0).UTC()
	default:
		return now.AddDate(0, 0, -30).UTC()
	}
}

// ---- usage insights ---------------------------------------------------------

// adminUsageInsights returns the rich payload that powers the Usage page: the
// per-provider routing breakdown, a bucketed activity-over-time series, recent
// activity rows, and headline metrics (success rate, average latency).

// blendedInputRate estimates the USD cost of a single input token for the
// period, derived from the tenant's own spend. It divides total spend by total
// tokens (prompt + completion) to get an average price per token. This is a
// deliberately conservative blended figure: savings happen on the input side,
// but pricing varies by provider/model and isn't stored per saved token, so a
// spend-weighted average grounds the estimate in what the tenant actually paid.
// Returns 0 when there is no usage to derive a rate from.
func blendedInputRate(sum schema.Summary) float64 {
	totalTokens := sum.PromptTokens + sum.CompletionTokens
	if totalTokens <= 0 || sum.CostMicros <= 0 {
		return 0
	}
	usd := float64(sum.CostMicros) / 1_000_000
	return usd / float64(totalTokens)
}

// adminModelUsage returns per-provider+model aggregate usage for the granular
// model usage table on the Usage page.

type timeBucket struct {
	label string
	count int64
}

// bucketTimeline applies time labels to the pre-bucketed SQL timeline points.
func bucketTimeline(points []schema.TimeBucket, from, to time.Time, n int) []timeBucket {
	if n <= 0 {
		n = 24
	}
	buckets := make([]timeBucket, n)
	span := to.Sub(from)
	if span <= 0 {
		span = time.Hour
	}
	slot := span / time.Duration(n)
	if slot <= 0 {
		slot = time.Minute
	}
	for i := 0; i < n; i++ {
		buckets[i].label = from.Add(time.Duration(i) * slot).Format("15:04")
	}
	for _, p := range points {
		if p.Bucket >= 0 && p.Bucket < n {
			buckets[p.Bucket].count = p.Count
		}
	}
	return buckets
}

// ---- quota tracker ----------------------------------------------------------

// adminQuotaUsage returns per-account usage so the Quota Tracker can show how
// much each connected account has consumed in the period.

// ---- usage SSE stream --------------------------------------------------------

// adminUsageStream serves an SSE endpoint that pushes usage events to the
// frontend for near-real-time dashboard updates. When a new usage record is
// inserted, the meter publishes to the usagehub.Hub, which delivers the event
// here. The frontend subscribes via EventSource and invalidates its query cache
// on each event.
func (s *Handler) adminUsageStream(w http.ResponseWriter, r *http.Request) {
	if s.usageHub == nil {
		WriteError(w, http.StatusServiceUnavailable, "usage hub not configured")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial heartbeat so the client knows the connection is live.
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Subscribe to usage events via buffered channel.
	listener := usagehub.NewListener(64)
	s.usageHub.Subscribe(listener)
	defer s.usageHub.Unsubscribe(listener)

	// Keepalive ping every 25s to prevent proxy timeouts.
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-listener.C:
			data, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// ---- console log ------------------------------------------------------------

// adminConsoleLog returns the buffered log lines from the console log.
func (s *Handler) adminConsoleLog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"logs": s.consoleLog.Entries()})
}

// ---- proxy pools ------------------------------------------------------------

var validProxyPoolTypes = map[string]bool{
	"http": true, "vercel": true, "cloudflare": true, "deno": true,
}

type proxyTestResult struct {
	status    string
	lastError string
	elapsedMS int64
}

// testProxyPoolConnectivity performs a real connectivity check against a proxy
// pool. For HTTP proxies it routes a GET httpbin.org/ip through the proxy; for
// relay types (vercel/cloudflare/deno) it sends relay headers.
func testProxyPoolConnectivity(pool schema.ProxyPool) proxyTestResult {
	timeout := 10 * time.Second

	switch pool.Type {
	case "vercel", "cloudflare", "deno":
		return testRelayPool(pool.ProxyURL, timeout)
	default: // "http"
		return testHTTPPool(pool.ProxyURL, timeout)
	}
}

func testHTTPPool(proxyURL string, timeout time.Duration) proxyTestResult {
	start := time.Now()
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return proxyTestResult{status: "error", lastError: "invalid proxy URL: " + err.Error()}
	}
	transport := &http.Transport{Proxy: http.ProxyURL(parsed)}
	client := &http.Client{Transport: transport, Timeout: timeout}
	req, err := http.NewRequestWithContext(context.Background(), "GET", "https://httpbin.org/ip", nil)
	if err != nil {
		return proxyTestResult{status: "error", lastError: err.Error()}
	}
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return proxyTestResult{status: "error", lastError: err.Error(), elapsedMS: elapsed}
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close //nolint:errcheck // best-effort close
	if resp.StatusCode >= 400 {
		return proxyTestResult{status: "error", lastError: fmt.Sprintf("proxy returned HTTP %d", resp.StatusCode), elapsedMS: elapsed}
	}
	return proxyTestResult{status: "active", elapsedMS: elapsed}
}

func testRelayPool(relayURL string, timeout time.Duration) proxyTestResult {
	start := time.Now()
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(context.Background(), "GET", relayURL, nil)
	if err != nil {
		return proxyTestResult{status: "error", lastError: err.Error()}
	}
	req.Header.Set("x-relay-target", "https://httpbin.org")
	req.Header.Set("x-relay-path", "/get")
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return proxyTestResult{status: "error", lastError: err.Error(), elapsedMS: elapsed}
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close //nolint:errcheck // best-effort close
	if resp.StatusCode >= 400 {
		return proxyTestResult{status: "error", lastError: fmt.Sprintf("relay returned HTTP %d", resp.StatusCode), elapsedMS: elapsed}
	}
	return proxyTestResult{status: "active", elapsedMS: elapsed}
}

// ---- skills -----------------------------------------------------------------

// skillsKey is the settings key under which skill toggles are stored. Skills
// are reusable system-prompt augmentations the gateway can apply.
const skillsKey = "skills"

type skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
}

func (s *Handler) loadSkills(r *http.Request) []skill {
	skills := []skill{}
	if s.settings == nil {
		return skills
	}
	raw, err := s.settings.Get(r.Context(), skillsKey)
	if err != nil || raw == "" {
		return skills
	}
	_ = json.Unmarshal([]byte(raw), &skills)
	return skills
}

func (s *Handler) saveSkills(r *http.Request, skills []skill) error {
	raw, err := json.Marshal(skills)
	if err != nil {
		return err
	}
	return s.settings.Set(r.Context(), skillsKey, string(raw))
}
