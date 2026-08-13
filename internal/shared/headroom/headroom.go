// Package headroom is an input-side token saver that compresses a request's
// messages by calling an external Headroom proxy (POST {url}/v1/compress) and
// replacing the messages with the compressed version.
//
// Like the slimmer package it operates on the canonical core.ChatRequest before
// any format translation, so a single implementation benefits every provider
// dialect. It is fail-open by design: any failure (network error, non-2xx
// status, invalid body, timeout, late response) leaves the request untouched
// and never propagates an error to the caller.
//
// It also detects "phantom savings": the proxy may report tokens_saved > 0
// while the outbound JSON payload does not actually shrink, so the provider
// still bills nearly the full amount. Such results are flagged and excluded
// from the accumulated savings.
//
// This file defines the pure, I/O-free types (Config, Stats). The HTTP client
// (Compressor) lives in compressor.go.
package headroom

import "time"

// Config controls headroom behavior for a request.
type Config struct {
	// Enabled turns the compressor on. When false the request is left untouched.
	Enabled bool
	// URL is the base proxy URL (without the /v1/compress suffix).
	URL string
	// CompressUserMessages, when true, asks the proxy to also compress user
	// messages (sent as config.compress_user_messages=true).
	CompressUserMessages bool
	// Timeout is the per-call deadline, resolved from headroom_timeout_ms.
	Timeout time.Duration
}

// Stats captures the per-request compression result for the meter.
//
// All byte/token fields are clamped to >= 0 by convention (see clamp).
type Stats struct {
	// TokensBefore is the proxy-reported token count before compression (>= 0).
	TokensBefore int
	// TokensAfter is the proxy-reported token count after compression (>= 0).
	TokensAfter int
	// TokensSaved is max(0, reported tokens_saved).
	TokensSaved int
	// BytesBefore is the JSON byte size of messages before compression (>= 0).
	BytesBefore int
	// BytesAfter is the JSON byte size of messages after compression (>= 0).
	BytesAfter int
	// BytesSaved is max(0, BytesBefore - BytesAfter).
	BytesSaved int
	// Phantom is true when phantom-savings were detected.
	Phantom bool
	// Compressed is true when the request messages were actually replaced.
	Compressed bool
}

// clamp returns a copy of s with every byte/token field forced to a
// non-negative value. BytesSaved is recomputed as max(0, BytesBefore-BytesAfter)
// and TokensSaved as max(0, TokensSaved) so a body that grows or a proxy that
// reports negative savings can never produce negative analytics.
func (s Stats) clamp() Stats {
	s.TokensBefore = max(0, s.TokensBefore)
	s.TokensAfter = max(0, s.TokensAfter)
	s.BytesBefore = max(0, s.BytesBefore)
	s.BytesAfter = max(0, s.BytesAfter)
	s.BytesSaved = max(0, s.BytesBefore-s.BytesAfter)
	s.TokensSaved = max(0, s.TokensSaved)
	return s
}
