package headroom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	core "github.com/bobbyunknown/flamegate/internal/domain"
)

// defaultMaxConcurrentCompress bounds how many compression calls may be in
// flight at once. The reference Headroom proxy defaults to a single worker with
// modest rpm/tpm limits, so an unbounded burst from an active coding session
// stampedes it into 503s (or crashes it). Capping concurrency keeps the proxy
// healthy; requests beyond the cap fail open (uncompressed) instead of piling
// on. Override with FLAMEGATE_HEADROOM_MAX_CONCURRENCY.
const defaultMaxConcurrentCompress = 4

// Compressor wraps a fail-open HTTP client and a logger. It compresses a
// request's messages by calling an external Headroom proxy and replacing the
// messages with the compressed version. Every failure path is fail-open: the
// request is left untouched and no error is propagated to the caller.
type Compressor struct {
	client *http.Client
	log    *logrus.Logger
	// sem bounds concurrent in-flight compression calls (see
	// defaultMaxConcurrentCompress).
	sem chan struct{}
}

// New constructs a Compressor. The HTTP client deliberately has NO client-level
// Timeout: the per-call deadline is enforced via context so a late response is
// abandoned through context cancellation rather than a transport-level timeout.
// A nil logger falls back to logrus.StandardLogger().
func New(log *logrus.Logger) *Compressor {
	if log == nil {
		log = logrus.StandardLogger()
	}
	return &Compressor{
		client: &http.Client{},
		log:    log,
		sem:    make(chan struct{}, resolveMaxConcurrency()),
	}
}

// resolveMaxConcurrency reads the concurrency cap from the environment, falling
// back to the default. Values below 1 are ignored.
func resolveMaxConcurrency() int {
	n := defaultMaxConcurrentCompress
	if v := strings.TrimSpace(os.Getenv("FLAMEGATE_HEADROOM_MAX_CONCURRENCY")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 1 {
			n = parsed
		}
	}
	return n
}

// compressRequest is the JSON body POSTed to Compress_Endpoint.
type compressRequest struct {
	Messages []openAIMessage `json:"messages"`
	Model    string          `json:"model"`
	Config   *compressConfig `json:"config,omitempty"`
}

// compressConfig carries the optional per-call proxy configuration.
type compressConfig struct {
	CompressUserMessages bool `json:"compress_user_messages"`
}

// compressResponse is the decoded proxy response. A nil/absent/empty Messages
// slice is treated as a failure (fail-open); only a non-empty array counts as a
// successful compression.
type compressResponse struct {
	Messages []openAIMessage `json:"messages"`
	Stats    *compressStats  `json:"stats"`
}

// compressStats mirrors the proxy-reported token statistics. Absent stats leave
// every token field at zero.
type compressStats struct {
	TokensBefore int `json:"tokens_before"`
	TokensAfter  int `json:"tokens_after"`
	TokensSaved  int `json:"tokens_saved"`
}

// Compress mutates req.Messages in place when compression succeeds and returns
// a Stats snapshot. It NEVER returns an error and NEVER panics out: every
// failure path is fail-open (req left unchanged) and logged at warning level
// with a masked URL.
func (c *Compressor) Compress(ctx context.Context, req *core.ChatRequest, cfg Config) *Stats {
	// Skip entirely when disabled or no URL is configured.
	if req == nil || !cfg.Enabled || strings.TrimSpace(cfg.URL) == "" {
		return &Stats{}
	}

	// Bound concurrent calls so a burst never stampedes (or crashes) a small
	// single-worker proxy. When every slot is busy, skip compression for this
	// request (fail-open) rather than pile on and trigger 503s.
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	default:
		c.log.Debug("headroom: concurrency cap reached; skipping compression (fail-open)")
		return &Stats{}
	}

	// Capture the outbound JSON size before the call so phantom-savings can be
	// judged against the real payload, not the proxy's token claim.
	bytesBefore := jsonBytes(toOpenAIMessages(req))

	resp, attempts, err := c.callCompress(ctx, req, cfg)
	if err != nil {
		c.logFailOpen(cfg.URL, attempts, err)
		return &Stats{}
	}
	if resp == nil || len(resp.Messages) == 0 {
		// Missing / null / non-array / empty messages -> fail-open.
		c.logFailOpen(cfg.URL, attempts, errors.New("response contained no compressed messages"))
		return &Stats{}
	}

	// Success: replace the request messages with the compressed mapping and
	// measure the resulting payload size.
	req.Messages = fromOpenAIMessages(resp.Messages)
	bytesAfter := jsonBytes(toOpenAIMessages(req))

	stats := Stats{
		BytesBefore: bytesBefore,
		BytesAfter:  bytesAfter,
		Compressed:  true,
	}
	if resp.Stats != nil {
		stats.TokensBefore = resp.Stats.TokensBefore
		stats.TokensAfter = resp.Stats.TokensAfter
		stats.TokensSaved = resp.Stats.TokensSaved
	}
	// Phantom detection: tokens claimed saved but the body did not shrink.
	stats.Phantom = isPhantom(bytesBefore, bytesAfter, defaultMinShrinkRatio)

	clamped := stats.clamp()
	return &clamped
}

// maxCompressAttempts bounds how many times callCompress will hit the proxy for
// a single request: one initial attempt plus retries. Headroom returns 503 (and
// occasionally 429/502/504) while its compression model is warming up or its
// async queue is saturated, expecting the client to retry shortly. A small
// bounded retry converts those transient failures into successful compressions
// instead of falling open and losing the savings.
const maxCompressAttempts = 3

// retryBackoff is the base delay between attempts when the proxy reports a
// transient status without a usable Retry-After header.
const retryBackoff = 250 * time.Millisecond

// callCompress performs the POST to Compress_Endpoint with a context deadline
// of cfg.Timeout. It returns the decoded response on success, plus the number
// of attempts made. It returns an error for any non-success condition so the
// caller can fail open; transient statuses (and transport errors) are retried
// within the same deadline before giving up.
func (c *Compressor) callCompress(ctx context.Context, req *core.ChatRequest, cfg Config) (*compressResponse, int, error) {
	body := compressRequest{
		Messages: toOpenAIMessages(req),
		Model:    req.Model,
	}
	if cfg.CompressUserMessages {
		body.Config = &compressConfig{CompressUserMessages: true}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}

	// The per-call deadline spans all attempts; a response arriving after it is
	// abandoned because client.Do returns a context error.
	callCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	endpoint := buildCompressEndpoint(cfg.URL)

	var lastErr error
	var retryAfter time.Duration
	attempts := 0
	for attempt := 0; attempt < maxCompressAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retrying, honoring Retry-After when present, but never
			// past the per-call deadline.
			if err := sleepCtx(callCtx, retryDelay(retryAfter)); err != nil {
				return nil, attempts, err
			}
		}
		attempts++

		decoded, after, retryable, err := c.doCompress(callCtx, endpoint, payload)
		if err == nil {
			return decoded, attempts, nil
		}
		lastErr = err
		retryAfter = after
		if !retryable {
			return nil, attempts, err
		}
	}
	return nil, attempts, lastErr
}

// doCompress performs a single POST attempt. It returns the decoded response on
// success, or an error plus whether the failure is transient (worth retrying)
// and any Retry-After hint parsed from the response.
func (c *Compressor) doCompress(ctx context.Context, endpoint string, payload []byte) (*compressResponse, time.Duration, bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, false, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		// Transport errors (connection refused, reset, transient DNS) are worth
		// one more try within the deadline; a permanent failure simply exhausts
		// the attempts and then fails open.
		return nil, 0, true, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode > 299 {
		retryable := isRetryableStatus(httpResp.StatusCode)
		var after time.Duration
		if retryable {
			after = parseRetryAfter(httpResp.Header.Get("Retry-After"))
			// Drain so the connection can be reused for the retry.
			_, _ = io.Copy(io.Discard, httpResp.Body)
		}
		return nil, after, retryable, fmt.Errorf("unexpected status %d", httpResp.StatusCode)
	}

	var decoded compressResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&decoded); err != nil {
		return nil, 0, false, err
	}
	return &decoded, 0, false, nil
}

// isRetryableStatus reports whether an HTTP status is a transient, retry-worthy
// signal from the proxy (busy / warming up / gateway hiccup).
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, // 429
		http.StatusBadGateway,         // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout:     // 504
		return true
	default:
		return false
	}
}

// retryDelay returns the delay to wait before the next attempt: the proxy's
// Retry-After hint when present and sane, otherwise the base backoff.
func retryDelay(retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	return retryBackoff
}

// parseRetryAfter parses a Retry-After header expressed in whole seconds. It
// ignores HTTP-date forms and caps the value at 2s so a misbehaving proxy can't
// stall the request near its deadline.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs <= 0 {
		return 0
	}
	d := time.Duration(secs) * time.Second
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	return d
}

// sleepCtx waits for d or until ctx is done, returning ctx.Err() if the
// deadline is reached first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// logFailOpen records a fail-open warning with a masked endpoint (never the raw
// URL, so embedded credentials and query strings stay out of logs). The work is
// wrapped in a recover so that a logging failure is itself suppressed and never
// escapes the pipeline.
func (c *Compressor) logFailOpen(rawURL string, attempts int, cause error) {
	defer func() { _ = recover() }()
	c.log.Warn("headroom compression failed; leaving request unchanged (fail-open)",
		"endpoint", maskURL(rawURL),
		"attempts", attempts,
		"error", cause,
	)
}
