// Package version exposes the FlameGate build version. It supports two sources,
// in priority order:
//
//  1. A value injected at build time via -ldflags "-X main.Version=...". This is
//     what release builds and `make build` use (derived from the git tag).
//  2. A committed VERSION file embedded into the binary. This is the fallback
//     for local builds (air, go run) where no ldflags are injected.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var embedded string

// Embedded returns the committed version string from the VERSION file, trimmed.
func Embedded() string {
	return strings.TrimSpace(embedded)
}

// Resolve picks the best available version for display. The ldflags-injected
// value wins when it is a real version; otherwise it falls back to the
// committed VERSION file, then to "dev".
func Resolve(injected string) string {
	injected = strings.TrimSpace(injected)
	if injected != "" && injected != "dev" {
		return injected
	}
	if e := Embedded(); e != "" {
		return e
	}
	if injected != "" {
		return injected
	}
	return "dev"
}

// IsDev reports whether the build is a development (unstamped) build.
// It checks the raw injected value, not the resolved display version.
// A dev build is one where no ldflags were injected (Version == "dev").
func IsDev(injected string) bool {
	return strings.TrimSpace(injected) == "dev" || strings.TrimSpace(injected) == ""
}
