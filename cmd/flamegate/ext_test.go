package main

import "testing"

func TestIsRemoteSource(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"store:codex", true},
		{"github:acme/codex@v0.2.0", true},
		{"url:https://x/codex.zip", true},
		{"https://x/codex.zip", true},
		{"./local-ext", false},
		{"/abs/path", false},
		{"my-ext", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isRemoteSource(c.in); got != c.want {
			t.Errorf("isRemoteSource(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestHasPrefixAny(t *testing.T) {
	if !hasPrefixAny("github:x/y", []string{"store:", "github:"}) {
		t.Error("expected github match")
	}
	if hasPrefixAny("foo", []string{"store:", "github:"}) {
		t.Error("unexpected match")
	}
}