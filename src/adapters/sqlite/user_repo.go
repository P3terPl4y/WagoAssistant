package sqlite

import (
	"App/src/domain"
	"context"
	"database/sql"
)

// UserRepo implements ports.UserRepository using SQLite.
type UserRepo struct {
	db *sql.DB
}

// NewUserRepo creates a new UserRepo.
func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) GetByID(ctx context.Context, id int) (*domain.User, error) {
	var u domain.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, email, phone, password_hash, role, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, email, phone, password_hash, role, created_at FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	var u domain.User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, username, email, phone, password_hash, role, created_at FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Username, &u.Email, &u.Phone, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) Create(ctx context.Context, username, email, phone, passwordHash string) (*domain.User, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (username, email, phone, password_hash) VALUES (?, ?, ?, ?)`,
		username, email, phone, passwordHash)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, int(id))
}

func (r *UserRepo) UpdatePassword(ctx context.Context, userID int, passwordHash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, userID)
	return err
}

func (r *UserRepo) UpdatePhone(ctx context.Context, userID int, phone string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET phone = ? WHERE id = ?`, phone, userID)
	return err
}

func (r *UserRepo) Delete(ctx context.Context, id int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (r *UserRepo) ListAll(ctx context.Context, limit string, offset string) ([]domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, username, email, phone, password_hash, role, created_at FROM users ORDER BY id`)
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

func (r *UserRepo) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count)
	return count, err
}

func (r *UserRepo) CheckDuplicate(ctx context.Context, username, email, phone string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE username = ? OR email = ? OR phone = ?`,
		username, email, phone).Scan(&count)
	return count > 0, err
}

func (r *UserRepo) CheckPhoneTaken(ctx context.Context, phone string, excludeUserID int) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE phone = ? AND id != ?`, phone, excludeUserID).Scan(&count)
	return count > 0, err
}
