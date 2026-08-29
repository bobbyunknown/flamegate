package extstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Downloader fetches remote artifacts into temporary files with a hard size
// limit, aborting oversized downloads so we never buffer a hostile payload.
type Downloader struct {
	httpc *http.Client
}

// NewDownloader builds a downloader with the supplied HTTP client.
func NewDownloader(httpc *http.Client) *Downloader {
	return &Downloader{httpc: httpc}
}

// FetchToTemp streams url to a temp file (prefix "extstore-"). Returns the temp
// path. On any failure the temp file is removed. maxBytes < 0 disables the cap.
func (d *Downloader) FetchToTemp(ctx context.Context, url string, maxBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("download: build request: %w", err)
	}
	res, err := d.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return "", fmt.Errorf("%w: %d %s", ErrHTTPWrap, res.StatusCode, res.Status)
	}

	tmp, err := os.CreateTemp("", "extstore-*.zip")
	if err != nil {
		return "", fmt.Errorf("download: create temp: %w", err)
	}
	abort := func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}

	w := io.Writer(tmp)
	var src io.Reader = res.Body
	if maxBytes >= 0 {
		src = io.LimitReader(res.Body, maxBytes+1)
	}
	n, err := io.Copy(w, src)
	if err != nil {
		abort()
		return "", fmt.Errorf("download: copy: %w", err)
	}
	if maxBytes >= 0 && n > maxBytes {
		abort()
		return "", ErrDownloadTooLarge
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("download: close: %w", err)
	}
	return tmp.Name(), nil
}