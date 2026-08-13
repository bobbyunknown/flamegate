package guardrails

import (
	"sync"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

// LogHub is a lightweight fan-out for newly-written audit log rows. It mirrors
// the usagehub.Hub pattern: the AuditWriter publishes after every successful
// flush, and the gateway's SSE endpoint subscribes to receive a near-real-time
// feed without polling the database.
type LogHub struct {
	mu        sync.RWMutex
	listeners map[*LogListener]struct{}
}

// LogListener receives audit log rows via a buffered channel. Slow listeners
// drop events instead of blocking the publisher.
type LogListener struct {
	C chan schema.GuardrailLog
}

// NewLogHub creates a LogHub.
func NewLogHub() *LogHub {
	return &LogHub{listeners: make(map[*LogListener]struct{})}
}

// NewLogListener creates a listener with the given buffer size.
func NewLogListener(bufSize int) *LogListener {
	if bufSize <= 0 {
		bufSize = 64
	}
	return &LogListener{C: make(chan schema.GuardrailLog, bufSize)}
}

// Subscribe registers a listener.
func (h *LogHub) Subscribe(l *LogListener) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.listeners[l] = struct{}{}
	h.mu.Unlock()
}

// Unsubscribe removes a listener.
func (h *LogHub) Unsubscribe(l *LogListener) {
	if h == nil {
		return
	}
	h.mu.Lock()
	delete(h.listeners, l)
	h.mu.Unlock()
}

// Publish broadcasts a row to all subscribers via non-blocking sends. Slow
// listeners drop the event rather than backpressure the audit writer.
func (h *LogHub) Publish(e schema.GuardrailLog) {
	if h == nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for l := range h.listeners {
		select {
		case l.C <- e:
		default:
		}
	}
}
