package postgre

import (
	"App/src/domain"
	"context"
	"database/sql"
)

// BotRepo implements ports.BotRepository using PostgreSQL.
type BotRepo struct {
	db *sql.DB
}

func NewBotRepo(db *sql.DB) *BotRepo {
	return &BotRepo{db: db}
}

func (r *BotRepo) GetByID(ctx context.Context, id int) (*domain.Bot, error) {
	var b domain.Bot
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, blocked, session_file, payment_status, created_at FROM bots WHERE id = $1`, id).
		Scan(&b.ID, &b.UserID, &b.Blocked, &b.SessionFile, &b.PaymentStatus, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &b, err
}

func (r *BotRepo) GetByUser(ctx context.Context, userID int) ([]domain.Bot, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, blocked, session_file, payment_status, created_at FROM bots WHERE user_id = $1`, userID)
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

	var botID int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO bots (user_id, session_file, payment_status) VALUES ($1, $2, $3) RETURNING id`,
		userID, sessionFile, paymentStatus).Scan(&botID)
	if err != nil {
		return 0, err
	}

	// Crear suscripción gratuita por defecto con expiración a 30 días (sintaxis PostgreSQL)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO subscriptions (bot_id, tier, msg_limit, expires_at) VALUES ($1, 'free', 10, CURRENT_DATE + INTERVAL '30 days')`,
		botID)
	if err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return botID, nil
}

func (r *BotRepo) UpdateBlocked(ctx context.Context, id int, blocked bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bots SET blocked = $1 WHERE id = $2`, blocked, id)
	return err
}

func (r *BotRepo) UpdatePaymentStatus(ctx context.Context, id int, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bots SET payment_status = $1 WHERE id = $2`, status, id)
	return err
}

func (r *BotRepo) UpdateSessionFile(ctx context.Context, id int, file string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bots SET session_file = $1 WHERE id = $2`, file, id)
	return err
}

func (r *BotRepo) Delete(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM bots WHERE id = $1`, id)
	return err
}

func (r *BotRepo) CountByUser(ctx context.Context, userID int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM bots WHERE user_id = $1`, userID).Scan(&count)
	return count, err
}
