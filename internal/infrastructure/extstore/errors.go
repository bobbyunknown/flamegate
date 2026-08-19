// Package extstore implements remote extension installation for FlameGate:
// source resolution, GitHub release discovery, download, tiered verification
// (checksum + Ed25519 signature), and sandboxed unpacking into the ExtDir.
package extstore

import "errors"

// Sentinel errors surfaced to CLI/admin/dashboard. Callers use errors.Is.
var (
	// ErrStoreIndexNotFound: store/index.json unreachable or unparseable.
	ErrStoreIndexNotFound = errors.New("store index not reachable (check wasm.store_index_url)")
	// ErrExtensionNotFound: slug absent from the store catalog.
	ErrExtensionNotFound = errors.New("extension not found in store")
	// ErrNoReleaseAsset: no matching release asset found for the tag-prefix.
	ErrNoReleaseAsset = errors.New("no release asset found")
	// ErrChecksumMismatch: SHA256 verification failed.
	ErrChecksumMismatch = errors.New("checksum mismatch")
	// ErrSignatureMissing: signature required but SHA256SUMS.sig absent.
	ErrSignatureMissing = errors.New("signature required but missing")
	// ErrSignatureInvalid: signature present but did not verify with known keys.
	ErrSignatureInvalid = errors.New("signature verification failed")
	// ErrUntrustedSource: strict mode or official denial because source unsigned.
	ErrUntrustedSource = errors.New("untrusted source")
	// ErrZipTraversal: archive contains symlink or path escaping the destination.
	ErrZipTraversal = errors.New("unsafe path or symlink in archive")
	// ErrDownloadTooLarge: response exceeded the configured size limit.
	ErrDownloadTooLarge = errors.New("download exceeds size limit")
	// ErrGitHubRateLimit: GitHub API returned 403/429 without a token.
	ErrGitHubRateLimit = errors.New("GitHub rate limit exceeded — set wasm.github_token_env")
	// ErrHTTPWrap: generic non-2xx upstream response.
	ErrHTTPWrap = errors.New("upstream request failed")
	// ErrBadSource: input could not be parsed into a known source kind.
	ErrBadSource = errors.New("invalid source")
)

// TrustLevel categorizes where an extension came from, driving UI badges.
type TrustLevel string

const (
	// TrustOfficial: store source with a valid baked-in signature.
	TrustOfficial TrustLevel = "official"
	// TrustCommunity: third-party source, checksum OK; signature optional.
	TrustCommunity TrustLevel = "community"
	// TrustLocal: installed from a local folder or multipart upload.
	TrustLocal TrustLevel = "local"
)