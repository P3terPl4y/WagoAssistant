package redis

import (
	"App/src/domain"
	"App/src/pkg/logger"
	"context"
	"encoding/json"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Cache implements ports.CacheService using Redis.
type Cache struct {
	client *goredis.Client
	logger logger.Logger
}

// Connect attempts to connect to Redis. Returns nil Cache if URL is empty or connection fails.
func Connect(redisURL string, log logger.Logger) *Cache {
	if redisURL == "" {
		log.Warn().Msg("REDIS_URL not configured, running without cache")
		return nil
	}
	opt, err := goredis.ParseURL(redisURL)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse REDIS_URL, running without cache")
		return nil
	}
	client := goredis.NewClient(opt)
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Error().Err(err).Msg("Failed to connect to Redis, running without cache")
		return nil
	}
	log.Info().Msg("Connected to Redis")
	return &Cache{client: client, logger: log.WithComponent("redis")}
}

// Client returns the underlying Redis client (for session storage).
func (c *Cache) Client() *goredis.Client {
	if c == nil {
		return nil
	}
	return c.client
}

// Available returns true if Redis is connected.
func (c *Cache) Available() bool {
	return c != nil && c.client != nil
}

func (c *Cache) chatKey(botID int, userJID string) string {
	return "chat:" + string(rune(botID)) + ":" + userJID
}

// We use a simple format function to avoid import fmt for just Sprintf
func chatKeyFmt(botID int, userJID string) string {
	// Use json marshal for int to string conversion to avoid fmt dependency
	b, _ := json.Marshal(botID)
	return "chat:" + string(b) + ":" + userJID
}

func (c *Cache) GetChatHistory(ctx context.Context, botID int, userJID string, limit int) ([]domain.ChatMessage, error) {
	if !c.Available() {
		return nil, nil
	}
	key := chatKeyFmt(botID, userJID)
	vals, err := c.client.LRange(ctx, key, -int64(limit), -1).Result()
	if err != nil || len(vals) == 0 {
		return nil, err
	}

	messages := make([]domain.ChatMessage, 0, len(vals))
	for _, v := range vals {
		var msg map[string]string
		if err := json.Unmarshal([]byte(v), &msg); err == nil {
			messages = append(messages, domain.ChatMessage{Role: msg["role"], Content: msg["content"]})
		}
	}
	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

func (c *Cache) SetChatHistory(ctx context.Context, botID int, userJID string, messages []domain.ChatMessage, ttl time.Duration) error {
	if !c.Available() {
		return nil
	}
	key := chatKeyFmt(botID, userJID)
	c.client.Del(ctx, key)
	for _, msg := range messages {
		msgBytes, _ := json.Marshal(map[string]string{"role": msg.Role, "content": msg.Content})
		c.client.RPush(ctx, key, string(msgBytes))
	}
	c.client.Expire(ctx, key, ttl)
	return nil
}

func (c *Cache) AppendChatMessage(ctx context.Context, botID int, userJID string, role, content string, maxHistory int64, ttl time.Duration) error {
	if !c.Available() {
		return nil
	}
	key := chatKeyFmt(botID, userJID)
	msgBytes, _ := json.Marshal(map[string]string{"role": role, "content": content})
	if err := c.client.RPush(ctx, key, string(msgBytes)).Err(); err != nil {
		c.logger.Warn().Err(err).Msg("Redis RPush failed")
		return err
	}
	c.client.LTrim(ctx, key, -maxHistory, -1)
	c.client.Expire(ctx, key, ttl)
	return nil
}

// TryLock attempts to acquire a distributed lock for the given key.
func (c *Cache) TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if !c.Available() {
		return false, nil
	}
	lockKey := "lock:" + key
	return c.client.SetNX(ctx, lockKey, "locked", ttl).Result()
}

// Unlock releases the distributed lock for the given key.
func (c *Cache) Unlock(ctx context.Context, key string) error {
	if !c.Available() {
		return nil
	}
	lockKey := "lock:" + key
	return c.client.Del(ctx, lockKey).Err()
}

// IncrementUsage increments the daily message count for a bot.
func (c *Cache) IncrementUsage(ctx context.Context, botID int) (int, error) {
	if !c.Available() {
		return 0, nil // Degrade gracefully
	}
	today := time.Now().Format("2006-01-02")
	b, _ := json.Marshal(botID)
	key := "usage:" + string(b) + ":" + today
	
	count, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		// First message of the day, set expiration to 24h + small buffer
		c.client.Expire(ctx, key, 25*time.Hour)
	}
	return int(count), nil
}

// GetUsage retrieves the daily message count for a bot.
func (c *Cache) GetUsage(ctx context.Context, botID int) (int, error) {
	if !c.Available() {
		return 0, nil
	}
	today := time.Now().Format("2006-01-02")
	b, _ := json.Marshal(botID)
	key := "usage:" + string(b) + ":" + today
	
	val, err := c.client.Get(ctx, key).Int()
	if err == goredis.Nil {
		return 0, nil
	}
	return val, err
}

// RecordGlobalMetric increments a platform-wide daily metric (e.g. "messages", "errors")
func (c *Cache) RecordGlobalMetric(ctx context.Context, metricType string) error {
	if !c.Available() {
		return nil
	}
	today := time.Now().Format("2006-01-02")
	key := "metric:" + metricType + ":" + today
	_, err := c.client.Incr(ctx, key).Result()
	if err == nil {
		c.client.Expire(ctx, key, 14*24*time.Hour) // Keep for 14 days
	}
	return err
}

// GetGlobalMetrics retrieves data for the past 'days' count (e.g. 7).
func (c *Cache) GetGlobalMetrics(ctx context.Context, metricType string, days int) ([]int, error) {
	if !c.Available() {
		return make([]int, days), nil
	}
	
	results := make([]int, days)
	now := time.Now()
	
	// Pipeline could be used here, but for 7 days simple gets are fine
	for i := 0; i < days; i++ {
		// Index 0 is 'days-1' ago, up to today
		date := now.AddDate(0, 0, -((days - 1) - i)).Format("2006-01-02")
		key := "metric:" + metricType + ":" + date
		
		val, err := c.client.Get(ctx, key).Int()
		if err == goredis.Nil {
			results[i] = 0
		} else if err == nil {
			results[i] = val
		} else {
			return nil, err
		}
	}
	return results, nil
}
