package extstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SourceKind is the classified origin of an install input.
type SourceKind string

const (
	// SourceStore: catalog slug, resolved via store/index.json.
	SourceStore SourceKind = "store"
	// SourceGitHub: github:owner/repo[@ref], resolved via GitHub API.
	SourceGitHub SourceKind = "github"
	// SourceURL: direct binary/zip URL.
	SourceURL SourceKind = "url"
	// SourceLocal: local folder containing schema.json + *.wasm.
	SourceLocal SourceKind = "local"
)

// SourceSpec is the normalized result of ParseSource. Only the fields relevant
// to the kind are populated.
type SourceSpec struct {
	Kind     SourceKind
	Slug     string // store:<slug>
	Owner    string // github:owner
	Repo     string // github:repo
	Ref      string // github:...@ref
	URL      string // url:<url>
	LocalDir string // local folder path
}

// ParseSource expands a user input string into a SourceSpec.
//
//	"codex"                          -> store, slug=codex
//	"store:codex"                    -> store, slug=codex
//	"github:acme/codex@v0.2.0"       -> github, owner=acme repo=codex ref=v0.2.0
//	"url:https://x/codex.zip"        -> url
//	"./my-ext" or "/abs/path"        -> local
func ParseSource(input string) (SourceSpec, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return SourceSpec{}, fmt.Errorf("%w: empty source", ErrBadSource)
	}

	switch {
	case strings.HasPrefix(in, "store:"):
		slug := strings.TrimPrefix(in, "store:")
		if slug == "" {
			return SourceSpec{}, fmt.Errorf("%w: store: requires slug", ErrBadSource)
		}
		return SourceSpec{Kind: SourceStore, Slug: slug}, nil

	case strings.HasPrefix(in, "github:"):
		rest := strings.TrimPrefix(in, "github:")
		ref := ""
		if i := strings.LastIndex(rest, "@"); i >= 0 {
			ref = rest[i+1:]
			rest = rest[:i]
		}
		// Repo names on GitHub never contain a slash; reject owner/repo/sub.
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return SourceSpec{}, fmt.Errorf("%w: github: expected owner/repo[@ref]", ErrBadSource)
		}
		return SourceSpec{Kind: SourceGitHub, Owner: parts[0], Repo: parts[1], Ref: ref}, nil

	case strings.HasPrefix(in, "url:"):
		u := strings.TrimPrefix(in, "url:")
		if u == "" {
			return SourceSpec{}, fmt.Errorf("%w: url: requires URL", ErrBadSource)
		}
		return SourceSpec{Kind: SourceURL, URL: u}, nil

	case strings.HasPrefix(in, "http://"), strings.HasPrefix(in, "https://"):
		return SourceSpec{Kind: SourceURL, URL: in}, nil

	case looksLikeLocalPath(in):
		return SourceSpec{Kind: SourceLocal, LocalDir: in}, nil

	default:
		// bare slug resolves through the store catalog
		return SourceSpec{Kind: SourceStore, Slug: in}, nil
	}
}

// looksLikeLocalPath reports whether input looks like a filesystem path rather
// than a store slug: relative dirs (./), absolute paths, or paths with a
// separator or extension.
func looksLikeLocalPath(s string) bool {
	if s == "." || s == ".." {
		return true
	}
	if s == "" || s[0] == '-' {
		return false
	}
	if filepath.IsAbs(s) {
		return true
	}
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	if strings.Contains(s, string(os.PathSeparator)) {
		return true
	}
	// foo/bar, foo.zip, foo.wasm — treat as local. A store slug never contains
	// a dot or a slash.
	return strings.ContainsAny(s, ".\\/")
}