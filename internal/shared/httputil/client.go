package httputil

import (
	"context"
	"net"
	"net/http"
	"time"
)

// RobustDialer returns a *net.Dialer with resilient DNS lookup.
func RobustDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 3 * time.Second}
				// Try 8.8.8.8 first, then 1.1.1.1, then system default
				if conn, err := d.DialContext(ctx, "udp", "8.8.8.8:53"); err == nil {
					return conn, nil
				}
				if conn, err := d.DialContext(ctx, "udp", "1.1.1.1:53"); err == nil {
					return conn, nil
				}
				return d.DialContext(ctx, network, address)
			},
		},
	}
}

// RobustTransport returns a pooled *http.Transport with resilient dialing.
func RobustTransport() *http.Transport {
	dialer := RobustDialer()
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
}

// NewClient returns an *http.Client configured with RobustTransport and the given timeout.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: RobustTransport(),
		Timeout:   timeout,
	}
}
