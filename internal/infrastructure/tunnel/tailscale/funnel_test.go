package tailscale

import (
	"encoding/json"
	"errors"
	"runtime"
	"testing"
)

func TestLoginURLRegex(t *testing.T) {
	line := "To authenticate, visit: https://login.tailscale.com/a/abc123XYZ now"
	got := loginURLRegex.FindString(line)
	want := "https://login.tailscale.com/a/abc123XYZ"
	if got != want {
		t.Fatalf("loginURLRegex = %q, want %q", got, want)
	}
	if loginURLRegex.MatchString("no url here") {
		t.Fatal("loginURLRegex matched a line with no URL")
	}
}

func TestEnableURLRegex(t *testing.T) {
	line := "enable Funnel at https://login.tailscale.com/f/funnel?node=xyz"
	got := enableURLRegex.FindString(line)
	want := "https://login.tailscale.com/f/funnel?node=xyz"
	if got != want {
		t.Fatalf("enableURLRegex = %q, want %q", got, want)
	}
}

func TestTsArgs(t *testing.T) {
	got := tsArgs("/data", "status", "--json")
	if runtime.GOOS == "windows" {
		if len(got) != 2 || got[0] != "status" || got[1] != "--json" {
			t.Fatalf("tsArgs (windows) = %v, want [status --json]", got)
		}
		return
	}
	// Unix prepends --socket <path>.
	if len(got) != 4 || got[0] != "--socket" || got[2] != "status" || got[3] != "--json" {
		t.Fatalf("tsArgs (unix) = %v, want [--socket <path> status --json]", got)
	}
	if got[1] != TailscaleSocket("/data") {
		t.Fatalf("tsArgs socket = %q, want %q", got[1], TailscaleSocket("/data"))
	}
}

func TestIsLoginError(t *testing.T) {
	loginErrs := []string{
		"backend in state NoState",
		"unexpected state: Stopped",
		"device is not logged in",
		"account Logged out",
		"NeedsLogin",
	}
	for _, msg := range loginErrs {
		if !isLoginError(errors.New(msg)) {
			t.Fatalf("isLoginError(%q) = false, want true", msg)
		}
	}
	if isLoginError(errors.New("connection refused")) {
		t.Fatal("isLoginError(unrelated) = true, want false")
	}
}

func TestDaemonStatusUnmarshal(t *testing.T) {
	raw := `{"BackendState":"Running","Self":{"Online":true,"DNSName":"host.tail1234.ts.net."},"AuthURL":""}`
	var s DaemonStatus
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.BackendState != "Running" {
		t.Fatalf("BackendState = %q", s.BackendState)
	}
	if s.Self == nil || !s.Self.Online {
		t.Fatal("Self.Online not parsed")
	}
	if s.Self.DNSName != "host.tail1234.ts.net." {
		t.Fatalf("DNSName = %q", s.Self.DNSName)
	}
}
