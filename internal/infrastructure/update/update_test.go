package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		name      string
		current   string
		candidate string
		want      bool
	}{
		{"patch bump", "v0.1.5", "v0.1.6", true},
		{"minor bump", "0.1.9", "0.2.0", true},
		{"major bump", "v1.2.3", "v2.0.0", true},
		{"equal", "v0.1.6", "v0.1.6", false},
		{"older candidate", "v0.2.0", "v0.1.9", false},
		{"mixed prefix", "0.1.6", "v0.1.7", true},
		{"dev current", "dev", "v0.1.6", false},
		{"dirty current", "v0.1.6-3-gabc1234-dirty", "v0.1.7", false},
		{"prerelease candidate ignored suffix", "v0.1.6", "v0.1.7-rc1", true},
		{"empty current", "", "v0.1.0", false},
		{"garbage candidate", "v0.1.6", "not-a-version", false},
		{"two-part version", "v1.0", "v1.1", true},
		{"double-digit patch bump", "v0.1.6", "v0.1.11", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNewer(tc.current, tc.candidate); got != tc.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tc.current, tc.candidate, got, tc.want)
			}
		})
	}
}

func TestCheckerFetchAndCache(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v0.2.0",
			Body:        "## What's New\n- Faster routing\n- Bug fixes",
			HTMLURL:     "https://github.com/bobbyunknown/flamegate/releases/tag/v0.2.0",
			PublishedAt: "2026-06-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	c := NewChecker("v0.1.0", "bobbyunknown/flamegate")
	// Point the checker at the test server by overriding fetch URL via a custom
	// client transport that rewrites the host.
	c.client = srv.Client()
	c.client.Transport = rewriteTransport{base: srv.URL, rt: srv.Client().Transport}

	info := c.Check(t.Context())
	if !info.Checked {
		t.Fatal("expected Checked=true")
	}
	if info.Latest != "v0.2.0" {
		t.Errorf("Latest = %q, want v0.2.0", info.Latest)
	}
	if !info.UpdateAvailable {
		t.Error("expected UpdateAvailable=true")
	}
	if info.Changelog == "" {
		t.Error("expected non-empty changelog")
	}

	// Second call should hit the cache, not the server.
	_ = c.Check(t.Context())
	if hits != 1 {
		t.Errorf("expected 1 server hit (cached second call), got %d", hits)
	}
}

// rewriteTransport redirects all requests to base, preserving the path.
type rewriteTransport struct {
	base string
	rt   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := http.NewRequestWithContext(req.Context(), req.Method, t.base+req.URL.Path, req.Body)
	if err != nil {
		return nil, err
	}
	target.Header = req.Header
	rt := t.rt
	if rt == nil {
		rt = http.DefaultTransport
	}
	return rt.RoundTrip(target)
}
