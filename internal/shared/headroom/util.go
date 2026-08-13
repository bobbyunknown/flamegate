package headroom

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

// credsRE matches "//user:pass@" (or "//user@") userinfo in a URL; urlQueryRE
// matches an http(s) URL up to (but excluding) its query string or fragment.
var (
	credsRE    = regexp.MustCompile(`//[^/@\s]+@`)
	urlQueryRE = regexp.MustCompile(`(https?://[^\s?#]+)[?#][^\s]*`)
)

// defaultMinShrinkRatio is the minimum fraction by which the outbound JSON body
// must shrink for reported savings to be considered real. When the body shrinks
// less than 5% the savings are treated as phantom.
const defaultMinShrinkRatio = 0.05

// compressPath is appended to the normalized base URL to form Compress_Endpoint.
const compressPath = "/v1/compress"

// buildCompressEndpoint normalizes base by dropping any fragment and one or more
// trailing slashes, then appends /v1/compress. The result ends in exactly one
// /v1/compress with no duplicate slashes.
func buildCompressEndpoint(base string) string {
	if i := strings.IndexByte(base, '#'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, "/")
	return base + compressPath
}

// maskURL strips credentials (userinfo) and the query string from raw, keeping
// only scheme, host, and path. It is used before logging an endpoint so secrets
// embedded in the configured URL never reach diagnostic logs.
// On a parse error it returns an empty string rather than risk leaking the raw
// value.
func maskURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	masked := url.URL{
		Scheme: u.Scheme,
		Opaque: u.Opaque,
		Host:   u.Host, // host:port only; userinfo lives in u.User and is dropped
		Path:   u.Path,
	}
	return masked.String()
}

// scrubURLText removes userinfo (credentials) and any query string/fragment
// from URLs embedded inside an arbitrary string (such as an error message), so
// secrets never leak through diagnostics surfaced to the UI or logs.
func scrubURLText(text string) string {
	text = credsRE.ReplaceAllString(text, "//")
	text = urlQueryRE.ReplaceAllString(text, "$1")
	return text
}

// jsonBytes returns the JSON-encoded byte size of v. On a marshal error it
// returns 0 so callers can treat it as a non-negative measurement.
func jsonBytes(v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(b)
}

// isPhantom reports whether reported token savings are phantom: the body did not
// shrink by at least minShrinkRatio. With minShrinkRatio == 0.05 the result is
// phantom when bytesAfter >= 0.95 * bytesBefore. It requires bytesBefore > 0;
// without a measurable "before" size no phantom judgment is made.
func isPhantom(bytesBefore, bytesAfter int, minShrinkRatio float64) bool {
	if bytesBefore <= 0 {
		return false
	}
	threshold := (1 - minShrinkRatio) * float64(bytesBefore)
	return float64(bytesAfter) >= threshold
}
