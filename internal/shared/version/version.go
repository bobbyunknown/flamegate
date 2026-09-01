// Package version exposes the FlameGate build version. It supports two sources,
// in priority order:
//
//  1. A value injected at build time via -ldflags "-X main.Version=...". This is
//     what release builds and `make build` use (derived from the git tag).
//  2. A committed VERSION file at the repository root read as fallback
//     for local builds (air, go run) where no ldflags are injected.
package version

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
)

var (
	embeddedOnce sync.Once
	embeddedVal  string
)

// Embedded returns the committed version string from the root VERSION file, trimmed.
func Embedded() string {
	embeddedOnce.Do(func() {
		// 1. Search for VERSION file from current working directory upwards (up to 5 levels)
		if dir, err := os.Getwd(); err == nil {
			curr := dir
			for i := 0; i < 5; i++ {
				candidate := filepath.Join(curr, "VERSION")
				if data, err := os.ReadFile(candidate); err == nil {
					val := strings.TrimSpace(string(data))
					if val != "" {
						embeddedVal = val
						return
					}
				}
				parent := filepath.Dir(curr)
				if parent == curr {
					break
				}
				curr = parent
			}
		}

		// 2. Fallback to Go build info VCS revision
		if info, ok := debug.ReadBuildInfo(); ok {
			if info.Main.Version != "" && info.Main.Version != "(devel)" {
				embeddedVal = info.Main.Version
				return
			}
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" && setting.Value != "" {
					rev := setting.Value
					if len(rev) > 7 {
						rev = rev[:7]
					}
					embeddedVal = rev
					return
				}
			}
		}
	})
	return embeddedVal
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
