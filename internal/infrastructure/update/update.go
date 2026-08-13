// Package update checks GitHub for newer FlameGate releases and exposes the
// latest version plus its changelog. It is read-only: it never downloads or
// applies anything. The result is cached in-memory to avoid hitting GitHub's
// API on every dashboard load.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// defaultRepo is the GitHub "owner/name" slug releases are fetched from.
const defaultRepo = "bobbyunknown/flamegate"

// defaultTTL is how long a successful check is cached before refreshing.
const defaultTTL = 15 * time.Minute

// Info is the update status reported to the dashboard.
type Info struct {
	// Current is the running build version (e.g. "v0.1.6" or "dev").
	Current string `json:"current"`
	// Latest is the newest release tag on GitHub, empty if the check failed.
	Latest string `json:"latest"`
	// UpdateAvailable is true when Latest is a strictly newer semver than Current.
	UpdateAvailable bool `json:"update_available"`
	// Changelog is the release body (markdown) for Latest.
	Changelog string `json:"changelog"`
	// PublishedAt is the RFC3339 publish time of the latest release.
	PublishedAt string `json:"published_at"`
	// HTMLURL links to the release page on GitHub.
	HTMLURL string `json:"html_url"`
	// Checked is true when GitHub was reached successfully (cache or live).
	Checked bool `json:"checked"`
}

// githubRelease is the subset of GitHub's release payload we consume.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

// Checker fetches and caches the latest release for a repository.
type Checker struct {
	current string
	repo    string
	client  *http.Client
	ttl     time.Duration

	mu       sync.Mutex
	cached   *Info
	cachedAt time.Time
}

// NewChecker builds a Checker for the running version. An empty repo defaults
// to the FlameGate repository.
func NewChecker(current, repo string) *Checker {
	if repo == "" {
		repo = defaultRepo
	}
	return &Checker{
		current: current,
		repo:    repo,
		client:  &http.Client{Timeout: 10 * time.Second},
		ttl:     defaultTTL,
	}
}

// Check returns the current update status, using the cached result when it is
// still fresh. On a network/API error it returns an Info describing the current
// version with Checked=false rather than failing the request.
func (c *Checker) Check(ctx context.Context) *Info {
	c.mu.Lock()
	if c.cached != nil && time.Since(c.cachedAt) < c.ttl {
		cached := *c.cached
		c.mu.Unlock()
		return &cached
	}
	c.mu.Unlock()

	info, err := c.fetch(ctx)
	if err != nil {
		// Serve a stale cache if we have one; it is better than nothing.
		c.mu.Lock()
		if c.cached != nil {
			cached := *c.cached
			c.mu.Unlock()
			return &cached
		}
		c.mu.Unlock()
		return &Info{Current: c.current, Checked: false}
	}

	c.mu.Lock()
	c.cached = info
	c.cachedAt = time.Now()
	c.mu.Unlock()

	out := *info
	return &out
}

// Refresh forces a live GitHub check, bypassing the cache, and stores the
// result. It backs the dashboard's "Check now" action so a freshly-published
// release shows up immediately instead of waiting for the cache TTL to expire.
// On a network/API error it serves a stale cache when available, otherwise it
// reports the current version with Checked=false.
func (c *Checker) Refresh(ctx context.Context) *Info {
	info, err := c.fetch(ctx)
	if err != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.cached != nil {
			cached := *c.cached
			return &cached
		}
		return &Info{Current: c.current, Checked: false}
	}

	c.mu.Lock()
	c.cached = info
	c.cachedAt = time.Now()
	c.mu.Unlock()

	out := *info
	return &out
}

// fetch performs the live GitHub API call and builds an Info.
func (c *Checker) fetch(ctx context.Context) (*Info, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", c.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "flamegate-update-checker")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("github releases: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("github releases: decode: %w", err)
	}

	info := &Info{
		Current:         c.current,
		Latest:          rel.TagName,
		Changelog:       strings.TrimSpace(rel.Body),
		PublishedAt:     rel.PublishedAt,
		HTMLURL:         rel.HTMLURL,
		Checked:         true,
		UpdateAvailable: IsNewer(c.current, rel.TagName),
	}
	return info, nil
}

// IsNewer reports whether candidate is a strictly newer semantic version than
// current. Non-release current versions ("dev", "-dirty" builds, unparseable
// tags) report false because there is no meaningful version to compare.
func IsNewer(current, candidate string) bool {
	cur, ok := parseSemver(current)
	if !ok {
		return false
	}
	cand, ok := parseSemver(candidate)
	if !ok {
		return false
	}
	return compareSemver(cand, cur) > 0
}

// semver is a parsed major.minor.patch triple. Pre-release and build metadata
// are ignored for comparison purposes — a tagged release always wins over a
// dirty/dev build, which parseSemver already rejects.
type semver struct {
	major, minor, patch int
}

// parseSemver parses "vX.Y.Z" or "X.Y.Z" (with optional -suffix) into a semver.
// It returns ok=false for "dev", dirty builds, or anything not shaped like a
// release tag.
func parseSemver(v string) (semver, bool) {
	v = strings.TrimSpace(v)
	if v == "" || v == "dev" {
		return semver{}, false
	}
	// Reject git-describe dirty builds (e.g. "v0.1.6-3-gabc1234-dirty").
	if strings.Contains(v, "dirty") {
		return semver{}, false
	}
	v = strings.TrimPrefix(v, "v")
	// Drop pre-release / build metadata after the core version.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return semver{}, false
	}
	out := semver{}
	dst := []*int{&out.major, &out.minor, &out.patch}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return semver{}, false
		}
		*dst[i] = n
	}
	return out, true
}

// compareSemver returns -1, 0, or 1 for a<b, a==b, a>b.
func compareSemver(a, b semver) int {
	if a.major != b.major {
		return cmpInt(a.major, b.major)
	}
	if a.minor != b.minor {
		return cmpInt(a.minor, b.minor)
	}
	return cmpInt(a.patch, b.patch)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
