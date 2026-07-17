// Package events implements a minimal in-memory pub/sub hub used to
// broadcast orchestrator state changes to WebSocket clients.
package events

import "sync"

// Event is the envelope broadcast to subscribers.
type Event struct {
	Type    string
	Payload any
}

// Hub is a simple fan-out broadcaster. It has no external dependencies.
type Hub struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewHub creates an empty Hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[chan Event]struct{})}
}

// Subscribe registers a new subscriber and returns a receive-only channel
// plus a cancel function that must be called to unsubscribe and release
// resources.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 16)

	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
	}

	return ch, cancel
}

// Publish broadcasts an event to every current subscriber. Slow subscribers
// that haven't drained their buffer are skipped rather than blocking the
// publisher.
func (h *Hub) Publish(e Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
