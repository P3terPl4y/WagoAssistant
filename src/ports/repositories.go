package ports

import (
	"App/src/domain"
	"context"

	"go.mau.fi/whatsmeow/types"
)

// UserRepository defines the contract for user data access.
type UserRepository interface {
	GetByID(ctx context.Context, id int) (*domain.User, error)
	GetUserByBotID(ctx context.Context, id int) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Create(ctx context.Context, username, email, phone, passwordHash string) (*domain.User, error)
	UpdatePassword(ctx context.Context, userID int, passwordHash string) error
	UpdatePhone(ctx context.Context, userID int, phone string) error
	Delete(ctx context.Context, id int) error
	ListAll(ctx context.Context, limit string, offset string) ([]domain.User, error)
	CountAdmins(ctx context.Context) (int, error)
	CheckDuplicate(ctx context.Context, username, email, phone string) (bool, error)
	CheckPhoneTaken(ctx context.Context, phone string, excludeUserID int) (bool, error)
}
type AdminRepository interface {
	NotifyAdmin(botID int, clientJID types.JID, msg string)
}

// BotRepository defines the contract for bot data access.
type BotRepository interface {
	GetByID(ctx context.Context, id int) (*domain.Bot, error)

	GetByUser(ctx context.Context, userID int) ([]domain.Bot, error)
	GetAll(ctx context.Context) ([]domain.Bot, error)
	Create(ctx context.Context, userID int, sessionFile, paymentStatus string) (int, error)
	UpdateBlocked(ctx context.Context, id int, blocked bool) error
	UpdatePaymentStatus(ctx context.Context, id int, status string) error
	UpdateSessionFile(ctx context.Context, id int, file string) error
	Delete(ctx context.Context, id int) error
	CountByUser(ctx context.Context, userID int) (int, error)
}

// PromptRepository defines the contract for bot prompt data access.
type PromptRepository interface {
	Get(ctx context.Context, botID int) (string, error)
	Save(ctx context.Context, botID int, prompt string) error
}

// SubscriptionRepository defines the contract for subscription data access.
type SubscriptionRepository interface {
	Get(ctx context.Context, botID int) (*domain.Subscription, error)
	Save(ctx context.Context, sub *domain.Subscription) error
}

// ChatRepository defines the contract for chat history data access.
type ChatRepository interface {
	SaveMessage(ctx context.Context, botID int, userJID, role, encryptedContent string) error
	GetHistory(ctx context.Context, botID int, userJID string, limit int) ([]domain.ChatMessage, error)
}

// OAuthRepository defines the contract for OAuth token storage.
type OAuthRepository interface {
	GetRefreshToken(ctx context.Context, userID int, provider string) (string, error)
	SaveRefreshToken(ctx context.Context, userID int, provider, refreshToken string) error
}
