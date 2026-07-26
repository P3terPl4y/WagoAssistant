package app

import (
	"App/src/domain"
	"App/src/pkg/logger"
	"App/src/ports"
	"context"
	"time"
)

// ChatService handles chat history with encryption and caching.
type ChatService struct {
	chats    ports.ChatRepository
	encrypt  ports.EncryptionService
	cache    ports.CacheService
	logger   logger.Logger
	maxHist  int64
	maxChars int
	cacheTTL time.Duration
}

func NewChatService(chats ports.ChatRepository, encrypt ports.EncryptionService, cache ports.CacheService, log logger.Logger, maxHistory int64, maxHistoryChars int) *ChatService {
	return &ChatService{chats: chats, encrypt: encrypt, cache: cache, logger: log.WithComponent("chat_service"), maxHist: maxHistory, maxChars: maxHistoryChars, cacheTTL: 1 * time.Hour}
}

func (s *ChatService) SaveMessage(ctx context.Context, botID int, userJID, role, content string) error {
	encrypted, err := s.encrypt.Encrypt(content)
	if err != nil {
		return err
	}
	if err := s.chats.SaveMessage(ctx, botID, userJID, role, encrypted); err != nil {
		return err
	}
	if s.cache != nil && s.cache.Available() {
		_ = s.cache.AppendChatMessage(ctx, botID, userJID, role, content, s.maxHist, s.cacheTTL)
	}
	return nil
}

func (s *ChatService) GetHistory(ctx context.Context, botID int, userJID string, limit int) ([]domain.ChatMessage, error) {
	if s.cache != nil && s.cache.Available() {
		cached, err := s.cache.GetChatHistory(ctx, botID, userJID, limit)
		if err == nil && len(cached) > 0 {
			return cached, nil
		}
	}
	messages, err := s.chats.GetHistory(ctx, botID, userJID, limit)
	if err != nil {
		return nil, err
	}
	for i := range messages {
		decrypted, err := s.encrypt.Decrypt(messages[i].Content)
		if err != nil {
			decrypted = messages[i].Content
		}
		messages[i].Content = decrypted
	}
	if s.cache != nil && s.cache.Available() && len(messages) > 0 {
		_ = s.cache.SetChatHistory(ctx, botID, userJID, messages, s.cacheTTL)
	}
	return messages, nil
}

func (s *ChatService) TruncateHistory(history []domain.ChatMessage) []domain.ChatMessage {
	total := 0
	start := len(history)
	for i := len(history) - 1; i >= 0; i-- {
		total += len(history[i].Content)
		if total > s.maxChars {
			break
		}
		start = i
	}
	return history[start:]
}
