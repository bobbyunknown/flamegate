package extstore

import (
	"testing"
)

func TestParseSource(t *testing.T) {
	tests := []struct {
		in   string
		want SourceSpec
	}{
		{"codex", SourceSpec{Kind: SourceStore, Slug: "codex"}},
		{"store:codex", SourceSpec{Kind: SourceStore, Slug: "codex"}},
		{"store:x-y-1", SourceSpec{Kind: SourceStore, Slug: "x-y-1"}},
		{"github:acme/codex@v0.2.0", SourceSpec{Kind: SourceGitHub, Owner: "acme", Repo: "codex", Ref: "v0.2.0"}},
		{"github:acme/codex", SourceSpec{Kind: SourceGitHub, Owner: "acme", Repo: "codex", Ref: ""}},
		{"url:https://x/codex.zip", SourceSpec{Kind: SourceURL, URL: "https://x/codex.zip"}},
		{"https://x/codex.zip", SourceSpec{Kind: SourceURL, URL: "https://x/codex.zip"}},
		{"./my-ext", SourceSpec{Kind: SourceLocal, LocalDir: "./my-ext"}},
		{"/abs/path", SourceSpec{Kind: SourceLocal, LocalDir: "/abs/path"}},
		{"my-ext", SourceSpec{Kind: SourceStore, Slug: "my-ext"}},
	}
	for _, tt := range tests {
		got, err := ParseSource(tt.in)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%q: got %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseSourceErrors(t *testing.T) {
	for _, in := range []string{"", "   ", "store:", "github:", "github:a/b/c/d", "url:"} {
		if _, err := ParseSource(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}