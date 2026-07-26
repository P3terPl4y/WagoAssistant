package postgre

import (
	"App/src/domain"
	"context"
	"database/sql"
)

// ChatRepo implements ports.ChatRepository using PostgreSQL.
type ChatRepo struct {
	db *sql.DB
}

func NewChatRepo(db *sql.DB) *ChatRepo {
	return &ChatRepo{db: db}
}

func (r *ChatRepo) SaveMessage(ctx context.Context, botID int, userJID, role, encryptedContent string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO chat_history (bot_id, user_jid, role, content) VALUES ($1, $2, $3, $4)`,
		botID, userJID, role, encryptedContent)
	return err
}

func (r *ChatRepo) GetHistory(ctx context.Context, botID int, userJID string, limit int) ([]domain.ChatMessage, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT role, content FROM chat_history WHERE bot_id = $1 AND user_jid = $2 ORDER BY created_at DESC LIMIT $3`,
		botID, userJID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.ChatMessage
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, err
		}
		messages = append(messages, domain.ChatMessage{Role: role, Content: content})
	}
	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}
