package domain

import "time"

// User represents a platform user (admin or regular).
type User struct {
	ID           int
	Username     string
	Email        string
	Phone        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

// Bot represents a WhatsApp bot instance owned by a user.
type Bot struct {
	ID            int
	UserID        int
	Username      string // denormalized for display
	Blocked       bool
	SessionFile   string
	PaymentStatus string // "free", "pending", "paid"
	CreatedAt     time.Time
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// Subscription represents the billing tier and usage limits for a bot.
type Subscription struct {
	BotID     int       `json:"bot_id"`
	Tier      string    `json:"tier"`      // 'free', 'pro', 'enterprise'
	MsgLimit  int       `json:"msg_limit"` // -1 for unlimited
	ExpiresAt time.Time `json:"expires_at"`
}

// Agenda represents a calendar appointment.
type Agenda struct {
	ID        int
	UserID    int
	Name      string
	Date      string
	Body      string
	CreatedAt time.Time
}

// BotStatus represents the runtime status of a bot.
type BotStatus string

const (
	BotStatusActive         BotStatus = "active"
	BotStatusInactive       BotStatus = "inactive"
	BotStatusQR             BotStatus = "qr"
	BotStatusPendingPayment BotStatus = "pending_payment"
	BotStatusError          BotStatus = "error"
)
