package middleware

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/sirupsen/logrus"
)

var (
	logMu         sync.Mutex
	httpLogWriter io.Writer
	llmLogWriter  io.Writer
)

// InitFileLogs initializes the dedicated log directory and truncates both
// http.log and llm.log so they always start fresh on server restart.
func InitFileLogs(logDir string) error {
	if logDir == "" {
		return nil
	}
	logMu.Lock()
	defer logMu.Unlock()

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("create log dir %s: %w", logDir, err)
	}

	httpFile, err := os.OpenFile(filepath.Join(logDir, "http.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open http.log: %w", err)
	}
	httpLogWriter = httpFile

	llmFile, err := os.OpenFile(filepath.Join(logDir, "llm.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open llm.log: %w", err)
	}
	llmLogWriter = llmFile

	return nil
}

// WriteHTTPLog appends a timestamped entry to http.log.
func WriteHTTPLog(entry string) {
	logMu.Lock()
	defer logMu.Unlock()
	if httpLogWriter != nil {
		timestamp := time.Now().Format(time.RFC3339)
		line := fmt.Sprintf("[%s] %s\n", timestamp, entry)
		_, _ = httpLogWriter.Write([]byte(line))
	}
}

// WriteLLMLog appends a timestamped entry to llm.log.
func WriteLLMLog(entry string) {
	logMu.Lock()
	defer logMu.Unlock()
	if llmLogWriter != nil {
		timestamp := time.Now().Format(time.RFC3339)
		line := fmt.Sprintf("[%s] %s\n", timestamp, entry)
		_, _ = llmLogWriter.Write([]byte(line))
	}
}

// RequestLogging logs each request.
// In the console/terminal logger, request logs are logged at Debug level so they
// do not pollute stdout in production (Info level).
// When file logging is enabled, requests (excluding noisy static assets)
// are written directly to http.log.
func RequestLogging(logger *logrus.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip logging for healthcheck probes to avoid noise
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}

			// Don't log static assets (js, css, png, svg, ico, web fonts)
			if isStaticAsset(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			latency := time.Since(start)

			fields := logrus.Fields{
				"method":  r.Method,
				"path":    r.URL.Path,
				"status":  ww.Status(),
				"latency": latency.String(),
			}

			if origin := r.Header.Get("Origin"); origin != "" {
				fields["origin"] = origin
			}
			fields["remote"] = r.RemoteAddr
			if ua := r.UserAgent(); ua != "" {
				fields["ua"] = ua
			}
			fields["bytes"] = ww.BytesWritten()

			// Terminal stdout logger: Debug level only (silent in prod, visible in dev/debug)
			logger.WithFields(fields).Debug("request")

			// File logger: write HTTP access log entry to http.log
			WriteHTTPLog(fmt.Sprintf("%s %s -> %d (%s, %d bytes) remote=%s",
				r.Method, r.URL.Path, ww.Status(), latency.String(), ww.BytesWritten(), r.RemoteAddr))
		})
	}
}

func isStaticAsset(path string) bool {
	if strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/providers/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".css", ".png", ".svg", ".ico", ".jpg", ".jpeg", ".webp", ".woff", ".woff2", ".ttf":
		return true
	}
	return false
}
