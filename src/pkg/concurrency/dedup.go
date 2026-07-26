package concurrency

import (
	"sync"
	"time"
)

// MessageDedup provides thread-safe deduplication of message IDs
// within a configurable time window.
type MessageDedup struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	window time.Duration
}

// NewMessageDedup creates a new deduplicator with the given window duration.
func NewMessageDedup(window time.Duration) *MessageDedup {
	return &MessageDedup{
		seen:   make(map[string]time.Time),
		window: window,
	}
}

// IsDuplicate returns true if the messageID was already seen within the window.
// It also prunes expired entries on each call.
func (d *MessageDedup) IsDuplicate(messageID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	// Prune expired entries
	for id, t := range d.seen {
		if now.Sub(t) > d.window {
			delete(d.seen, id)
		}
	}

	if _, exists := d.seen[messageID]; exists {
		return true
	}
	d.seen[messageID] = now
	return false
}
