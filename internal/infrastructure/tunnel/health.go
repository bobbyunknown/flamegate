package tunnel

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// HealthConfig holds timing parameters for health checks.
type HealthConfig struct {
	Interval     time.Duration // Poll interval
	Timeout      time.Duration // Total timeout
	FetchTimeout time.Duration // Per-fetch timeout
	DNSTimeout   time.Duration // DNS resolution timeout
}

// CloudflareHealthConfig is the health check config for Cloudflare tunnels.
// DNS propagates fast, so short timeouts are OK.
var CloudflareHealthConfig = HealthConfig{
	Interval:     2 * time.Second,
	Timeout:      60 * time.Second,
	FetchTimeout: 5 * time.Second,
	DNSTimeout:   2 * time.Second,
}

// TailscaleHealthConfig is the health check config for Tailscale funnels.
// Cert provisioning + *.ts.net DNS propagation is slower, so longer timeouts.
var TailscaleHealthConfig = HealthConfig{
	Interval:     2 * time.Second,
	Timeout:      180 * time.Second,
	FetchTimeout: 8 * time.Second,
	DNSTimeout:   3 * time.Second,
}

// ProbeURLAlive checks if a URL is reachable by first resolving its hostname
// via DNS, then making an HTTP GET to {url}/healthz.
func ProbeURLAlive(rawURL string, cfg HealthConfig) bool {
	if rawURL == "" {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return false
	}

	// DNS check.
	if !ResolveDNS(hostname, cfg.DNSTimeout) {
		return false
	}

	// HTTP check.
	healthURL := rawURL + "/healthz"
	ctx, cancel := context.WithTimeout(context.Background(), cfg.FetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// WaitForHealth polls a URL until it responds to health checks or the timeout
// is reached. The cancel channel can be used to abort early.
func WaitForHealth(rawURL string, cfg HealthConfig, cancel <-chan struct{}) error {
	deadline := time.After(cfg.Timeout)
	for {
		select {
		case <-cancel:
			return fmt.Errorf("canceled")
		case <-deadline:
			return fmt.Errorf("health check timeout after %v", cfg.Timeout)
		default:
		}
		if ProbeURLAlive(rawURL, cfg) {
			return nil
		}
		time.Sleep(cfg.Interval)
	}
}

// CheckInternet does a TCP connectivity check to 1.1.1.1:443 to verify
// internet connectivity is available.
func CheckInternet() bool {
	conn, err := net.DialTimeout("tcp", "1.1.1.1:443", 3*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
