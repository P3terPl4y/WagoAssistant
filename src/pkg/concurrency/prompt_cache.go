package concurrency

import (
	"sync"
	"time"
)

// PromptCache provides in-memory caching of bot system prompts with TTL.
type PromptCache struct {
	mu      sync.RWMutex
	entries map[int]promptEntry
	ttl     time.Duration
}

type promptEntry struct {
	value     string
	expiresAt time.Time
}

// NewPromptCache creates a new cache with the given TTL.
func NewPromptCache(ttl time.Duration) *PromptCache {
	return &PromptCache{
		entries: make(map[int]promptEntry),
		ttl:     ttl,
	}
}

// Get returns the cached prompt for the given botID if not expired.
func (c *PromptCache) Get(botID int) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[botID]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.value, true
}

// Set stores a prompt in the cache with the configured TTL.
func (c *PromptCache) Set(botID int, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[botID] = promptEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes a prompt from the cache.
func (c *PromptCache) Invalidate(botID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, botID)
}
