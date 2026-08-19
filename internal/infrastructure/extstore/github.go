package extstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Asset is a downloadable asset attached to a GitHub release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Release is a matching GitHub release resolved by tag prefix.
type Release struct {
	Tag         string
	PublishedAt time.Time
	Assets      []Asset
}

// githubRelease is the wire shape of a single element from GET /repos/{o}/{r}/releases.
type githubRelease struct {
	Tag         string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Assets      []Asset `json:"assets"`
}

// GithubClient resolves extension release assets from GitHub Releases with an
// in-memory TTL cache so dashboard listing does not exhaust the API quota.
type GithubClient struct {
	httpc   *http.Client
	baseURL string // default: https://api.github.com; overridable for tests
	token   func() string
	cache   *TTLCache
	ttl     time.Duration
}

// NewGithubClient builds a client. token returns the optional GITHUB_TOKEN at
// request time (never logged). baseURL defaults to https://api.github.com.
func NewGithubClient(httpc *http.Client, token func() string, cache *TTLCache, ttl time.Duration) *GithubClient {
	return &GithubClient{httpc: httpc, baseURL: "https://api.github.com", token: token, cache: cache, ttl: ttl}
}

// LatestRelease returns the newest release whose tag begins with tagPrefix and
// whose asset matches assetPattern. assetPattern may contain {version}, which is
// substituted from the version portion of the tag after removing tagPrefix.
// Results are cached under an owner/repo/tagPrefix key for ttl.
func (g *GithubClient) LatestRelease(ctx context.Context, owner, repo, tagPrefix, assetPattern string) (*Release, error) {
	key := fmt.Sprintf("rel:%s/%s:%s", owner, repo, tagPrefix)
	if v, ok := g.cache.Get(key); ok {
		return v.(*Release), nil
	}

	rel, err := g.fetchLatest(ctx, owner, repo, tagPrefix, assetPattern)
	if err != nil {
		return nil, err
	}
	g.cache.Set(key, rel, g.ttl)
	return rel, nil
}

func (g *GithubClient) fetchLatest(ctx context.Context, owner, repo, tagPrefix, assetPattern string) (*Release, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100", g.baseURL, owner, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != nil {
		if tok := g.token(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}

	res, err := g.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: request: %w", err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, ErrGitHubRateLimit
	case http.StatusNotFound:
		return nil, fmt.Errorf("github: %w: repo %s/%s not found or private", ErrHTTPWrap, owner, repo)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("%w: %d %s", ErrHTTPWrap, res.StatusCode, strings.TrimSpace(string(body)))
	}

	var releases []githubRelease
	if err := json.NewDecoder(res.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("github: decode releases: %w", err)
	}

	// Find the newest published release whose tag starts with the prefix.
	var best *Release
	for i := range releases {
		if !strings.HasPrefix(releases[i].Tag, tagPrefix) {
			continue
		}
		rel, match := releaseFromGitHub(releases[i], tagPrefix, assetPattern)
		if !match {
			continue
		}
		if best == nil || rel.PublishedAt.After(best.PublishedAt) {
			best = rel
		}
	}
	if best == nil {
		return nil, fmt.Errorf("github: %w: tag prefix %q in %s/%s", ErrNoReleaseAsset, tagPrefix, owner, repo)
	}
	return best, nil
}

func releaseFromGitHub(rel githubRelease, tagPrefix, assetPattern string) (*Release, bool) {
	version := strings.TrimPrefix(rel.Tag, tagPrefix)
	needle := strings.ReplaceAll(assetPattern, "{version}", version)
	var matched []Asset
	for _, a := range rel.Assets {
		if a.Name == needle {
			matched = append(matched, a)
		}
	}
	if len(matched) == 0 {
		return nil, false
	}
	published, _ := time.Parse(time.RFC3339, rel.PublishedAt)
	return &Release{Tag: rel.Tag, PublishedAt: published, Assets: matched}, true
}