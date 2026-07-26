package config

import (
	"encoding/hex"
	"os"
	"time"
)

// Config holds all application configuration.
// Every value has a sensible default so the server starts without any .env file.
type Config struct {
	Environment string
	ServerAddr  string
	BaseURL     string

	DatabaseURL   string
	RedisURL      string
	EncryptionKey []byte

	AdminUsername string
	AdminEmail    string
	AdminPhone    string
	AdminPass     string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	AI AIConfig

	MaxBots              int
	MaxConnectRetries    int
	SubscriptionDuration time.Duration
	MaxHistory           int64
	MaxHistoryChars      int
	SessionExpiration    time.Duration
	RateLimitPerMinute   int
	MaxPromptLength      int
	MaxMsgLength         int
	AITimeoutTotal       time.Duration
	DedupWindow          time.Duration

	CookieSecure bool
}

type AIConfig struct {
	OpenRouterKey string
	OpenRouterURL string
	LegacyKey     string
	LegacyURL     string
	LocalURL      string
	LocalEnabled  bool
	FreeModels    []string
}

// Load builds a Config. All values have defaults; environment variables override them.
func Load() *Config {
	cfg := &Config{
		Environment:   env("APP_ENV", "production"),
		ServerAddr:    env("SERVER_ADDR", "0.0.0.0:3000"),
		BaseURL:       env("BASE_URL", "https://wago.redcliente.cl"),
		DatabaseURL:   env("DATABASE_URL", "postgres://gowa:go-wa333P3ter*@localhost:5432/wago?sslmode=disable"),
		RedisURL:      os.Getenv("REDIS_URL"),
		AdminUsername: env("ADMIN_USERNAME", "admin"),
		AdminEmail:    env("ADMIN_EMAIL", "admin@wago.cl"),
		AdminPhone:    env("ADMIN_PHONE", "+5351652038"),
		AdminPass:     env("ADMIN_PASSWORD", "admin123"),

		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),

		AI: AIConfig{
			OpenRouterKey: env("OPENROUTER_API_KEY", ""),
			OpenRouterURL: env("OPENROUTER_URL", "https://openrouter.ai/api/v1/chat/completions"),
			LegacyKey:     env("LEGACY_API_KEY", ""),
			LegacyURL:     env("LEGACY_URL", "https://apifreellm.com/api/v1/chat"),
			LocalURL:      env("LOCAL_AI_URL", "http://localhost:8080/v1/chat/completions"),
			LocalEnabled:  os.Getenv("LOCAL_AI_ENABLED") == "true",
			FreeModels:    []string{"openrouter/free"},
		},

		MaxBots:              50,
		MaxConnectRetries:    5,
		SubscriptionDuration: 7 * 24 * time.Hour,
		MaxHistory:           8,
		MaxHistoryChars:      2000,
		SessionExpiration:    1 * time.Hour,
		RateLimitPerMinute:   10,
		MaxPromptLength:      2000,
		MaxMsgLength:         500,
		AITimeoutTotal:       40 * time.Second,
		DedupWindow:          3 * time.Second,

		// false by default → works on plain HTTP without issues
		CookieSecure: os.Getenv("COOKIE_SECURE") == "true",
	}

	// Encryption key: default value baked in so .env is not required
	keyHex := env("ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		panic("ENCRYPTION_KEY must be a 64-char hex string (32 bytes)")
	}
	cfg.EncryptionKey = key

	return cfg
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
