package proxy

import "testing"

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		if got := humanBytes(tt.n); got != tt.want {
			t.Errorf("humanBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		ms   int
		want string
	}{
		{0, "0ms"},
		{999, "999ms"},
		{1000, "1.0s"},
		{1500, "1.5s"},
		{59000, "59.0s"},
		{60000, "1m 0s"},
		{90000, "1m 30s"},
		{125000, "2m 5s"},
	}
	for _, tt := range tests {
		if got := humanDuration(tt.ms); got != tt.want {
			t.Errorf("humanDuration(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

func TestHumanInt(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{45119, "45,119"},
		{1000000, "1,000,000"},
		{-45119, "-45,119"},
		{-1000, "-1,000"},
	}
	for _, tt := range tests {
		if got := humanInt(tt.n); got != tt.want {
			t.Errorf("humanInt(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1); got != "" {
		t.Errorf("plural(1) = %q, want empty", got)
	}
	for _, n := range []int{0, 2, 10, -1} {
		if got := plural(n); got != "s" {
			t.Errorf("plural(%d) = %q, want \"s\"", n, got)
		}
	}
}
