package handlers

import (
	"sync/atomic"
	"time"
)

// TimeoutNotifier holds atomic timeout values that can be updated at runtime
// from the dashboard settings. The pipeline reads these per-request to get
// dynamic timeouts without database lookups.
//
// Zero value is safe to use: all timeouts default to 0 (no limit).
type TimeoutNotifier struct {
	streamStall    atomic.Int64 // nanoseconds
	responseHeader atomic.Int64 // nanoseconds
	request        atomic.Int64 // nanoseconds
}

// NewTimeoutNotifier creates a notifier with the given initial timeouts.
func NewTimeoutNotifier(stall, responseHeader, request time.Duration) *TimeoutNotifier {
	tn := &TimeoutNotifier{}
	tn.streamStall.Store(int64(stall))
	tn.responseHeader.Store(int64(responseHeader))
	tn.request.Store(int64(request))
	return tn
}

// NotifyTimeouts updates all three timeout values atomically.
// Called from the settings handler after a dashboard save.
func (tn *TimeoutNotifier) NotifyTimeouts(stall, responseHeader, request time.Duration) {
	tn.streamStall.Store(int64(stall))
	tn.responseHeader.Store(int64(responseHeader))
	tn.request.Store(int64(request))
}

// StreamStallTimeout returns the current stream stall timeout.
func (tn *TimeoutNotifier) StreamStallTimeout() time.Duration {
	return time.Duration(tn.streamStall.Load())
}

// ResponseHeaderTimeout returns the current response header timeout.
func (tn *TimeoutNotifier) ResponseHeaderTimeout() time.Duration {
	return time.Duration(tn.responseHeader.Load())
}

// RequestTimeout returns the current request timeout.
func (tn *TimeoutNotifier) RequestTimeout() time.Duration {
	return time.Duration(tn.request.Load())
}
