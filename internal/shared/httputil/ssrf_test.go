package httputil

import "testing"

func TestValidateBaseURL_AllowPrivate(t *testing.T) {
	// Default: loopback/private blocked.
	if err := ValidateBaseURL("http://127.0.0.1:11434/v1"); err == nil {
		t.Fatalf("expected loopback to be blocked by default")
	}
	if err := ValidateBaseURL("http://192.168.1.10:8080"); err == nil {
		t.Fatalf("expected RFC1918 to be blocked by default")
	}
	if err := ValidateBaseURL("http://172.16.21.122:18000"); err == nil {
		t.Fatalf("expected RFC1918 172.16/12 to be blocked by default")
	}

	SetAllowPrivateBaseURL(true)
	t.Cleanup(func() { SetAllowPrivateBaseURL(false) })

	// Loopback + private now permitted.
	if err := ValidateBaseURL("http://127.0.0.1:11434/v1"); err != nil {
		t.Errorf("loopback should pass with allow flag: %v", err)
	}
	if err := ValidateBaseURL("http://192.168.1.10:8080"); err != nil {
		t.Errorf("RFC1918 should pass with allow flag: %v", err)
	}
	if err := ValidateBaseURL("http://172.16.21.122:18000"); err != nil {
		t.Errorf("RFC1918 172.16/12 should pass with allow flag: %v", err)
	}
	if err := ValidateBaseURL("http://[::1]:8080"); err != nil {
		t.Errorf("IPv6 loopback should pass with allow flag: %v", err)
	}

	// Cloud metadata + link-local + unspecified + bad scheme stay blocked.
	for _, bad := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://metadata.google.internal/",
		"http://169.254.1.1/",
		"http://0.0.0.0/",
		"file:///etc/passwd",
		"http://0x7f000001/",
	} {
		if err := ValidateBaseURL(bad); err == nil {
			t.Errorf("expected %q to remain blocked with allow flag", bad)
		}
	}
}

func TestValidateOAuthRedirectURI(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty", "", false},
		{"localhost http", "http://localhost:3000/callback", false},
		{"localhost https", "https://localhost:8443/auth/callback", false},
		{"127.0.0.1", "http://127.0.0.1:3000/callback", false},
		{"127.0.0.2", "http://127.0.0.2:3000/callback", false},
		{"IPv6 loopback", "http://[::1]:3000/callback", false},
		{"no scheme", "localhost:3000/callback", true},
		{"file scheme", "file:///etc/passwd", true},
		{"private 10.x", "http://10.0.0.1:3000/callback", true},
		{"private 192.168.x", "http://192.168.1.1:3000/callback", true},
		{"private 172.16.x", "http://172.16.0.1:3000/callback", true},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data", true},
		{"link-local", "http://169.254.1.1:3000/callback", true},
		{"unspecified", "http://0.0.0.0:3000/callback", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOAuthRedirectURI(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateOAuthRedirectURI(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
