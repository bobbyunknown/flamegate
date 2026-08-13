package proxy

import (
	"net/http"

	"github.com/bobbyunknown/flamegate/internal/domain/shared"
)

// extractForwardedHeaders returns a map of safe-listed client headers
// extracted from h. Returns nil if no safe-listed headers are present.
func extractForwardedHeaders(h http.Header) map[string]string {
	var out map[string]string
	for _, hdr := range shared.ForwardedHeaders {
		if v := h.Get(hdr); v != "" {
			if out == nil {
				out = make(map[string]string)
			}
			out[hdr] = v
		}
	}
	return out
}
