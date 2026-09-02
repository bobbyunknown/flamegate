package connectors

import "strings"

// tier1Slugs are the custom template providers that remain natively connected because they
// use DirectStreamable (zero-copy streaming via io.ReadCloser) which cannot
// be implemented inside WASM.
var tier1Slugs = []string{"custom-openai", "custom-anthropic", "custom-gemini"}

// IsTier1Provider reports whether provider should stay as a native
// connector rather than being routed through WASM. Matching is case-
// insensitive.
func IsTier1Provider(slug string) bool {
	lower := strings.ToLower(slug)
	for _, s := range tier1Slugs {
		if s == lower {
			return true
		}
	}
	return false
}
