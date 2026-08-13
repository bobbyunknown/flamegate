package version

import "testing"

func TestResolve(t *testing.T) {
	embed := Embedded()

	tests := []struct {
		name     string
		injected string
		want     string
	}{
		{"injected real version wins", "v1.2.3", "v1.2.3"},
		{"injected trims whitespace", "  v2.0.0 ", "v2.0.0"},
		{"empty falls back to embedded", "", embed},
		{"whitespace-only falls back to embedded", "   ", embed},
		{"dev falls back to embedded", "dev", embed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.injected)
			want := tt.want
			if want == "" {
				want = "dev"
			}
			if got != want {
				t.Fatalf("Resolve(%q) = %q, want %q", tt.injected, got, want)
			}
		})
	}
}

func TestResolveNeverEmpty(t *testing.T) {
	if got := Resolve(""); got == "" {
		t.Fatal("Resolve(\"\") returned empty string, want non-empty")
	}
}

func TestIsDev(t *testing.T) {
	tests := []struct {
		injected string
		want     bool
	}{
		{"dev", true},
		{"", true},
		{"  ", true},
		{"v1.0.0", false},
		{"v0.1.24-3-gabcdef", false},
	}
	for _, tt := range tests {
		t.Run(tt.injected, func(t *testing.T) {
			if got := IsDev(tt.injected); got != tt.want {
				t.Fatalf("IsDev(%q) = %v, want %v", tt.injected, got, tt.want)
			}
		})
	}
}

func TestEmbeddedTrimmed(t *testing.T) {
	e := Embedded()
	if e != "" {
		if e[0] == ' ' || e[len(e)-1] == ' ' || e[len(e)-1] == '\n' {
			t.Fatalf("Embedded() not trimmed: %q", e)
		}
	}
}
