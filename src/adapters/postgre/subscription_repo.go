package postgre

import (
	"App/src/domain"
	"context"
	"database/sql"
)

// SubscriptionRepo implements ports.SubscriptionRepository.
type SubscriptionRepo struct {
	db *sql.DB
}

func NewSubscriptionRepo(db *sql.DB) *SubscriptionRepo {
	return &SubscriptionRepo{db: db}
}

func (r *SubscriptionRepo) Get(ctx context.Context, botID int) (*domain.Subscription, error) {
	var sub domain.Subscription
	sub.BotID = botID
	err := r.db.QueryRowContext(ctx,
		`SELECT tier, msg_limit, expires_at FROM subscriptions WHERE bot_id = $1`, botID).
		Scan(&sub.Tier, &sub.MsgLimit, &sub.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &sub, err
}

func (r *SubscriptionRepo) Save(ctx context.Context, sub *domain.Subscription) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO subscriptions (bot_id, tier, msg_limit, expires_at) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (bot_id) DO UPDATE SET tier = $5, msg_limit = $6, expires_at = $7`,
		sub.BotID, sub.Tier, sub.MsgLimit, sub.ExpiresAt,
		sub.Tier, sub.MsgLimit, sub.ExpiresAt)
	return err
}

// PromptRepo implements ports.PromptRepository.
type PromptRepo struct {
	db *sql.DB
}

func NewPromptRepo(db *sql.DB) *PromptRepo {
	return &PromptRepo{db: db}
}

func (r *PromptRepo) Get(ctx context.Context, botID int) (string, error) {
	var prompt string
	err := r.db.QueryRowContext(ctx,
		`SELECT prompt FROM prompts WHERE bot_id = $1`, botID).Scan(&prompt)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return prompt, err
}

func (r *PromptRepo) Save(ctx context.Context, botID int, prompt string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO prompts (bot_id, prompt) VALUES ($1, $2) ON CONFLICT (bot_id) DO UPDATE SET prompt = $3`,
		botID, prompt, prompt)
	return err
}

// OAuthRepo implements ports.OAuthRepository.
type OAuthRepo struct {
	db *sql.DB
}

func NewOAuthRepo(db *sql.DB) *OAuthRepo {
	return &OAuthRepo{db: db}
}

func (r *OAuthRepo) GetRefreshToken(ctx context.Context, userID int, provider string) (string, error) {
	var token string
	err := r.db.QueryRowContext(ctx,
		`SELECT refresh_token FROM oauth_tokens WHERE user_id = $1 AND provider = $2`,
		userID, provider).Scan(&token)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return token, err
}

func (r *OAuthRepo) SaveRefreshToken(ctx context.Context, userID int, provider, refreshToken string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO oauth_tokens (user_id, provider, refresh_token, updated_at) VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		 ON CONFLICT (user_id, provider) DO UPDATE SET refresh_token = $4, updated_at = CURRENT_TIMESTAMP`,
		userID, provider, refreshToken, refreshToken)
	return err
}
