package postgre

import (
	"App/src/domain"
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepo implements ports.UserRepository using PostgreSQL.
type UserRepo struct {
	db *pgxpool.Pool
}

func NewUserRepo(db *pgxpool.Pool) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) GetByID(ctx context.Context, id int) (*domain.User, error) {
	var u domain.User
	err := r.db.QueryRow(ctx,
		`SELECT id, username, email, phone, password_hash, role, created_at FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	err := r.db.QueryRow(ctx,
		`SELECT id, username, email, phone, password_hash, role, created_at FROM users WHERE username = $1`, username).
		Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := r.db.QueryRow(ctx,
		`SELECT id, username, email, phone, password_hash, role, created_at FROM users WHERE email = $1`, email).
		Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) Create(ctx context.Context, username, email, phone, passwordHash string) (*domain.User, error) {
	var id int
	err := r.db.QueryRow(ctx,
		`INSERT INTO users (username, email, phone, password_hash) VALUES ($1, $2, $3, $4) RETURNING id`,
		username, email, phone, passwordHash).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}
func (r *UserRepo) CreateHistoryPedidos(ctx context.Context, userID int, client string, pedido string) (*domain.Pedido, error) {
	var p = new(domain.Pedido)
	err := r.db.QueryRow(ctx,
		`INSERT INTO pedidos (user_id, client, pedido) VALUES ($1, $2, $3) RETURNING id`,
		userID, client, pedido).Scan(&p.ID)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}
	return p, nil
}
func (r *UserRepo) UpdatePassword(ctx context.Context, userID int, passwordHash string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, passwordHash, userID)
	return err
}
func (r *UserRepo) UpdateEmail(ctx context.Context, userID int, email string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET email = $1 WHERE id = $2`, email, userID)
	return err
}
func (r *UserRepo) UpdatePhone(ctx context.Context, userID int, phone string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET phone = $1 WHERE id = $2`, phone, userID)
	return err
}

func (r *UserRepo) Delete(ctx context.Context, id int) error {
	_, err := r.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

func (r *UserRepo) ListAll(ctx context.Context, limit string, offset string) ([]domain.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, username, email, phone, password_hash, role, created_at FROM users ORDER BY id OFFSET $1 LIMIT $2`,
		offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []domain.User
	for rows.Next() {
		var u domain.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.PasswordHash, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}
func (r *UserRepo) ListAllHistory(ctx context.Context, userID int, limit string, offset string) ([]domain.Pedido, error) {
	rows, err := r.db.Query(ctx,
		`SELECT * FROM pedidos WHERE user_id=$1 ORDER BY id OFFSET $2 LIMIT $3`,
		userID, offset, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pedido []domain.Pedido
	for rows.Next() {
		var p domain.Pedido
		if err := rows.Scan(&p.ID, &p.UserID, &p.Client, &p.Text, &p.CreatedAt); err != nil {
			return nil, err
		}
		pedido = append(pedido, p)
	}
	return pedido, nil
}
func (r *UserRepo) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count)
	return count, err
}

func (r *UserRepo) CheckDuplicate(ctx context.Context, username, email, phone string) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE username = $1 OR email = $2 OR phone = $3`, username, email, phone).Scan(&count)
	return count > 0, err
}

func (r *UserRepo) CheckPhoneTaken(ctx context.Context, phone string, excludeUserID int) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE phone = $1 AND id != $2`, phone, excludeUserID).Scan(&count)
	return count > 0, err
}

func (r *UserRepo) GetUserByBotID(ctx context.Context, botID int) (*domain.User, error) {
	var u domain.User
	var userID int
	err := r.db.QueryRow(ctx,
		`SELECT user_id FROM bots WHERE id = $1`, botID).Scan(&userID)
	if err != nil {
		return nil, err
	}
	err = r.db.QueryRow(ctx,
		`SELECT id, username, email, phone, password_hash, role, created_at FROM users WHERE id = $1`, userID).
		Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}
