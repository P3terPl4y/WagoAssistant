package sqlite

import (
	"App/src/domain"
	"context"
	"database/sql"
)

// BotRepo implements ports.BotRepository using SQLite.
type BotRepo struct {
	db *sql.DB
}

// NewBotRepo creates a new BotRepo.
func NewBotRepo(db *sql.DB) *BotRepo {
	return &BotRepo{db: db}
}

func (r *BotRepo) GetByID(ctx context.Context, id int) (*domain.Bot, error) {
	var b domain.Bot
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, blocked, session_file, payment_status, created_at FROM bots WHERE id = ?`, id).
		Scan(&b.ID, &b.UserID, &b.Blocked, &b.SessionFile, &b.PaymentStatus, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &b, err
}

func (r *BotRepo) GetByUser(ctx context.Context, userID int) ([]domain.Bot, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, blocked, session_file, payment_status, created_at FROM bots WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bots []domain.Bot
	for rows.Next() {
		var b domain.Bot
		if err := rows.Scan(&b.ID, &b.UserID, &b.Blocked, &b.SessionFile, &b.PaymentStatus, &b.CreatedAt); err != nil {
			return nil, err
		}
		bots = append(bots, b)
	}
	return bots, nil
}

func (r *BotRepo) GetAll(ctx context.Context) ([]domain.Bot, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, blocked, session_file, payment_status, created_at FROM bots ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bots []domain.Bot
	for rows.Next() {
		var b domain.Bot
		if err := rows.Scan(&b.ID, &b.UserID, &b.Blocked, &b.SessionFile, &b.PaymentStatus, &b.CreatedAt); err != nil {
			return nil, err
		}
		bots = append(bots, b)
	}
	return bots, nil
}

func (r *BotRepo) Create(ctx context.Context, userID int, sessionFile, paymentStatus string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO bots (user_id, session_file, payment_status) VALUES (?, ?, ?)`,
		userID, sessionFile, paymentStatus)
	if err != nil {
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Create default free subscription valid for 30 days
	_, err = tx.ExecContext(ctx,
		`INSERT INTO subscriptions (bot_id, tier, msg_limit, expires_at) VALUES (?, 'free', 10, datetime('now', '+30 days'))`,
		id)
	if err != nil {
		return 0, err
	}

	return int(id), tx.Commit()
}

func (r *BotRepo) UpdateBlocked(ctx context.Context, id int, blocked bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bots SET blocked = ? WHERE id = ?`, blocked, id)
	return err
}

func (r *BotRepo) UpdatePaymentStatus(ctx context.Context, id int, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bots SET payment_status = ? WHERE id = ?`, status, id)
	return err
}

func (r *BotRepo) UpdateSessionFile(ctx context.Context, id int, file string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bots SET session_file = ? WHERE id = ?`, file, id)
	return err
}

func (r *BotRepo) Delete(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM bots WHERE id = ?`, id)
	return err
}

func (r *BotRepo) CountByUser(ctx context.Context, userID int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bots WHERE user_id = ?`, userID).Scan(&count)
	return count, err
}
