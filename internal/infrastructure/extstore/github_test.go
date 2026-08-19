package extstore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestLatestRelease(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.URL.Path != "/repos/acme/codex/releases" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`[
			{"tag_name":"codex-v0.1.0","published_at":"2026-07-01T00:00:00Z",
			 "assets":[{"name":"codex-0.1.0.zip","browser_download_url":"https://x/codex-0.1.0.zip","size":10}]},
			{"tag_name":"codex-v0.2.0","published_at":"2026-08-01T00:00:00Z",
			 "assets":[{"name":"codex-0.2.0.zip","browser_download_url":"https://x/codex-0.2.0.zip","size":42}]},
			{"tag_name":"other-v9","published_at":"2026-08-02T00:00:00Z",
			 "assets":[{"name":"other-9.zip","browser_download_url":"https://x/other-9.zip","size":1}]}
		]`))
	}))
	defer srv.Close()

	c := NewGithubClient(srv.Client(), nil, NewTTLCache(), time.Minute)
	c.baseURL = srv.URL

	rel, err := c.LatestRelease(context.Background(), "acme", "codex", "codex-v", "codex-{version}.zip")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "codex-v0.2.0" {
		t.Fatalf("got newest tag %q, want codex-v0.2.0", rel.Tag)
	}
	if len(rel.Assets) != 1 || rel.Assets[0].Size != 42 {
		t.Fatalf("assets = %+v", rel.Assets)
	}
	if !rel.PublishedAt.Equal(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("PublishedAt = %v", rel.PublishedAt)
	}
}

// TestLatestReleaseCacheHit ensures a second call does not hit the API.
func TestLatestReleaseCacheHit(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`[
			{"tag_name":"x-v1","published_at":"2026-08-01T00:00:00Z",
			 "assets":[{"name":"x-1.zip","browser_download_url":"https://x/x-1.zip","size":5}]}
		]`))
	}))
	defer srv.Close()

	c := NewGithubClient(srv.Client(), nil, NewTTLCache(), time.Minute)
	c.baseURL = srv.URL
	ctx := context.Background()

	if _, err := c.LatestRelease(ctx, "acme", "x", "x-v", "x-{version}.zip"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.LatestRelease(ctx, "acme", "x", "x-v", "x-{version}.zip"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("cache miss: %d API hits, want 1", got)
	}
}

func TestLatestReleaseNoAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"codex-v0.2.0","published_at":"2026-08-01T00:00:00Z","assets":[]}]`))
	}))
	defer srv.Close()

	c := NewGithubClient(srv.Client(), nil, NewTTLCache(), time.Minute)
	c.baseURL = srv.URL
	_, err := c.LatestRelease(context.Background(), "acme", "codex", "codex-v", "codex-{version}.zip")
	if err == nil {
		t.Fatal("expected ErrNoReleaseAsset")
	}
}

func TestLatestReleaseRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewGithubClient(srv.Client(), nil, NewTTLCache(), time.Minute)
	c.baseURL = srv.URL
	_, err := c.LatestRelease(context.Background(), "acme", "codex", "codex-v", "codex-{version}.zip")
	if err != ErrGitHubRateLimit {
		t.Fatalf("err = %v, want ErrGitHubRateLimit", err)
	}
}