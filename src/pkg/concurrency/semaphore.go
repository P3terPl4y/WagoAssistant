package concurrency

import (
	"App/src/ports"
	"context"
	"sync"
	"time"
)

// UserSemaphore prevents concurrent processing of messages from the same user.
// This avoids duplicate AI responses when users send messages rapidly.
type UserSemaphore struct {
	mu     sync.Mutex
	active map[string]bool
	cache  ports.CacheService
}

// NewUserSemaphore creates a new UserSemaphore.
func NewUserSemaphore(cache ports.CacheService) *UserSemaphore {
	return &UserSemaphore{
		active: make(map[string]bool),
		cache:  cache,
	}
}

// TryLock attempts to acquire a lock for the given key.
// Returns true if the lock was acquired, false if already held.
func (s *UserSemaphore) TryLock(key string) bool {
	if s.cache != nil && s.cache.Available() {
		// Use distributed lock if cache is available
		acquired, err := s.cache.TryLock(context.Background(), key, 30*time.Second)
		if err == nil {
			return acquired
		}
	}

	// Fallback to in-memory lock
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[key] {
		return false
	}
	s.active[key] = true
	return true
}

// Unlock releases the lock for the given key.
func (s *UserSemaphore) Unlock(key string) {
	if s.cache != nil && s.cache.Available() {
		_ = s.cache.Unlock(context.Background(), key)
		return
	}

	// Fallback to in-memory unlock
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.active, key)
}
