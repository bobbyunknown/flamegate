package handlers

import (
	"testing"
	"time"
)

func TestTimeoutNotifierZeroValue(t *testing.T) {
	var tn TimeoutNotifier
	if tn.StreamStallTimeout() != 0 || tn.ResponseHeaderTimeout() != 0 || tn.RequestTimeout() != 0 {
		t.Fatal("zero-value TimeoutNotifier should report zero timeouts")
	}
}

func TestNewTimeoutNotifier(t *testing.T) {
	tn := NewTimeoutNotifier(5*time.Second, 10*time.Second, 30*time.Second)
	if got := tn.StreamStallTimeout(); got != 5*time.Second {
		t.Errorf("StreamStallTimeout = %v, want 5s", got)
	}
	if got := tn.ResponseHeaderTimeout(); got != 10*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 10s", got)
	}
	if got := tn.RequestTimeout(); got != 30*time.Second {
		t.Errorf("RequestTimeout = %v, want 30s", got)
	}
}

func TestNotifyTimeoutsUpdates(t *testing.T) {
	tn := NewTimeoutNotifier(time.Second, time.Second, time.Second)
	tn.NotifyTimeouts(2*time.Second, 4*time.Second, 8*time.Second)
	if got := tn.StreamStallTimeout(); got != 2*time.Second {
		t.Errorf("StreamStallTimeout = %v, want 2s", got)
	}
	if got := tn.ResponseHeaderTimeout(); got != 4*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 4s", got)
	}
	if got := tn.RequestTimeout(); got != 8*time.Second {
		t.Errorf("RequestTimeout = %v, want 8s", got)
	}
}

func TestTimeoutNotifierConcurrent(t *testing.T) {
	// Exercise the atomics under concurrent read/write to catch races
	// (run with -race).
	tn := NewTimeoutNotifier(time.Second, time.Second, time.Second)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			tn.NotifyTimeouts(time.Duration(i), time.Duration(i), time.Duration(i))
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		_ = tn.StreamStallTimeout()
		_ = tn.ResponseHeaderTimeout()
		_ = tn.RequestTimeout()
	}
	<-done
}
