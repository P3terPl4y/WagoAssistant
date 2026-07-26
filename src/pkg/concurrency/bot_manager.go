package concurrency

import (
	"App/src/pkg/logger"
	"context"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
)

// ManagedBot holds the runtime state of an active bot.
type ManagedBot struct {
	Client *whatsmeow.Client
	Cancel context.CancelFunc
}

// BotManager provides thread-safe management of active bot instances,
// including registration, status checking, and graceful shutdown.
type BotManager struct {
	mu     sync.RWMutex
	bots   map[int]*ManagedBot
	logger logger.Logger
}

// En el paquete concurrency o en el adaptador de AI
type RateLimiter struct {
	mu          sync.Mutex
	lastCall    time.Time
	minInterval time.Duration
}

func (r *RateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()
	elapsed := time.Since(r.lastCall)
	if elapsed < r.minInterval {
		time.Sleep(r.minInterval - elapsed)
	}
	r.lastCall = time.Now()
}

// NewBotManager creates a new BotManager.
func NewBotManager(log logger.Logger) *BotManager {
	return &BotManager{
		bots:   make(map[int]*ManagedBot),
		logger: log.WithComponent("bot_manager"),
	}
}

// Register adds a bot to the active registry.
func (m *BotManager) Register(botID int, client *whatsmeow.Client, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bots[botID] = &ManagedBot{Client: client, Cancel: cancel}
	m.logger.Info().Int("bot_id", botID).Msg("Bot registered")
}

// Unregister removes a bot from the active registry.
func (m *BotManager) Unregister(botID int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bots, botID)
	m.logger.Info().Int("bot_id", botID).Msg("Bot unregistered")
}

// IsActive returns true if the bot is in the active registry.
func (m *BotManager) IsActive(botID int) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.bots[botID]
	return ok
}

// GetClient returns the WhatsApp client for a bot, or nil if not active.
func (m *BotManager) GetClient(botID int) *whatsmeow.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if b, ok := m.bots[botID]; ok {
		return b.Client
	}
	return nil
}

// ActiveIDs returns a list of all active bot IDs.
func (m *BotManager) ActiveIDs() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]int, 0, len(m.bots))
	for id := range m.bots {
		ids = append(ids, id)
	}
	return ids
}

// ShutdownAll gracefully disconnects all bots within the given timeout.
func (m *BotManager) ShutdownAll(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info().Int("count", len(m.bots)).Msg("Shutting down all bots")

	var wg sync.WaitGroup
	for id, bot := range m.bots {
		wg.Add(1)
		go func(botID int, b *ManagedBot) {
			defer wg.Done()
			m.logger.Info().Int("bot_id", botID).Msg("Disconnecting bot")
			b.Cancel()
			if b.Client != nil {
				b.Client.Disconnect()
			}
		}(id, bot)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info().Msg("All bots disconnected cleanly")
	case <-time.After(timeout):
		m.logger.Warn().Msg("Bot shutdown timed out, some bots may not have disconnected cleanly")
	}

	m.bots = make(map[int]*ManagedBot)
}
