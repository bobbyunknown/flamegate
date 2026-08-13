package cloudflare

import "testing"

func TestURLRegexMatches(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantURL  string
		wantHost string
	}{
		{
			name:     "boxed log line",
			line:     "2024-01-01 |  https://happy-cloud-brown-fox.trycloudflare.com  |",
			wantURL:  "https://happy-cloud-brown-fox.trycloudflare.com",
			wantHost: "happy-cloud-brown-fox",
		},
		{
			name:     "leading whitespace",
			line:     "   https://one-two-three.trycloudflare.com",
			wantURL:  "https://one-two-three.trycloudflare.com",
			wantHost: "one-two-three",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := urlRegex.FindStringSubmatch(tt.line)
			if m == nil {
				t.Fatalf("no match for %q", tt.line)
			}
			if m[1] != tt.wantURL {
				t.Fatalf("URL = %q, want %q", m[1], tt.wantURL)
			}
			if m[2] != tt.wantHost {
				t.Fatalf("host = %q, want %q", m[2], tt.wantHost)
			}
		})
	}
}

func TestURLRegexRejectsBareDomain(t *testing.T) {
	// The hostname must contain a hyphen; the bare marketing domain must not match.
	if urlRegex.MatchString("visit https://trycloudflare.com for info") {
		t.Fatal("urlRegex matched bare trycloudflare.com")
	}
	// A single-label host with no hyphen must not match.
	if urlRegex.MatchString("https://api.trycloudflare.com") {
		t.Fatal("urlRegex matched hyphen-less host")
	}
}
