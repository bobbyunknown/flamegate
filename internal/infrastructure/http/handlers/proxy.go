package handlers

import "sync/atomic"

// ProxyNotifier holds atomic outbound proxy configuration that can be updated
// at runtime from the dashboard settings. The dispatcher reads these per-request
// to apply a global proxy fallback without database lookups.
//
// Zero value is safe to use: proxy is disabled and URL is empty.
type ProxyNotifier struct {
	enabled atomic.Bool
	url     atomic.Value // string
	noProxy atomic.Value // string
}

// NewProxyNotifier creates a notifier with the given initial proxy settings.
func NewProxyNotifier(enabled bool, proxyURL, noProxy string) *ProxyNotifier {
	pn := &ProxyNotifier{}
	pn.enabled.Store(enabled)
	pn.url.Store(proxyURL)
	pn.noProxy.Store(noProxy)
	return pn
}

// NotifyProxy updates all proxy values atomically.
// Called from the settings handler after a dashboard save.
func (pn *ProxyNotifier) NotifyProxy(enabled bool, proxyURL, noProxy string) {
	pn.enabled.Store(enabled)
	pn.url.Store(proxyURL)
	pn.noProxy.Store(noProxy)
}

// ProxyURL returns the current proxy URL, or "" when disabled or unset.
func (pn *ProxyNotifier) ProxyURL() string {
	if !pn.enabled.Load() {
		return ""
	}
	if v := pn.url.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// NoProxy returns the current comma-separated bypass list.
func (pn *ProxyNotifier) NoProxy() string {
	if v := pn.noProxy.Load(); v != nil {
		return v.(string)
	}
	return ""
}
