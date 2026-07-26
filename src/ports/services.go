package ports

import (
	"App/src/domain"
	"context"
	"time"
)

// AIService defines the contract for AI text generation.
type AIService interface {
	// Call sends a prompt to an AI provider and returns the response text.
	Call(ctx context.Context, prompt string) (string, error)
}

// EncryptionService defines the contract for encrypting/decrypting content.
type EncryptionService interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// CacheService defines the contract for caching chat history.
type CacheService interface {
	GetChatHistory(ctx context.Context, botID int, userJID string, limit int) ([]domain.ChatMessage, error)
	SetChatHistory(ctx context.Context, botID int, userJID string, messages []domain.ChatMessage, ttl time.Duration) error
	AppendChatMessage(ctx context.Context, botID int, userJID string, role, content string, maxHistory int64, ttl time.Duration) error
	TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, key string) error
	IncrementUsage(ctx context.Context, botID int) (int, error)
	GetUsage(ctx context.Context, botID int) (int, error)
	RecordGlobalMetric(ctx context.Context, metricType string) error
	GetGlobalMetrics(ctx context.Context, metricType string, days int) ([]int, error)
	Available() bool
}
