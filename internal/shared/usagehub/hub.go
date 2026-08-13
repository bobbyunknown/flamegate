// Package usagehub provides a lightweight pub/sub hub that notifies
// subscribers when new usage records are inserted. The gateway uses this
// to push SSE events to the Quota and Usage pages for near-real-time
// dashboard updates without polling.
package usagehub

import (
	"sync"
)

// Event is a minimal usage event broadcast to subscribers after a usage
// record is persisted. It carries just enough data for the frontend to
// know which query keys to invalidate.
type Event struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	AccountID string `json:"account_id"`
	Tokens    int    `json:"tokens"`
}

// Listener receives usage events via a buffered channel. The channel-based
// approach ensures that slow listeners (e.g. SSE clients with network hiccups)
// never block the publisher (which runs in the hot request path).
type Listener struct {
	C chan Event // Buffered channel for non-blocking delivery.
}

// NewListener creates a listener with a buffered event channel.
// Events are dropped when the buffer is full (slow consumer).
func NewListener(bufSize int) *Listener {
	if bufSize <= 0 {
		bufSize = 64
	}
	return &Listener{C: make(chan Event, bufSize)}
}

// Hub manages subscribers and broadcasts usage events.
type Hub struct {
	mu        sync.RWMutex
	listeners map[*Listener]struct{}
}

// New creates a Hub.
func New() *Hub {
	return &Hub{
		listeners: make(map[*Listener]struct{}),
	}
}

// Subscribe registers a listener. Call Unsubscribe when done.
func (h *Hub) Subscribe(l *Listener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listeners[l] = struct{}{}
}

// Unsubscribe removes a listener.
func (h *Hub) Unsubscribe(l *Listener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.listeners, l)
}

// Publish broadcasts an event to all subscribers via non-blocking channel
// sends. If a listener's channel is full (slow consumer), the event is
// silently dropped for that listener — this prevents a single slow SSE client
// from blocking all API request processing.
func (h *Hub) Publish(ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for l := range h.listeners {
		select {
		case l.C <- ev:
		default:
			// Drop event for slow listener — non-blocking.
		}
	}
}
