package postgre

import (
	"App/src/pkg/logger"
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"

	_ "github.com/lib/pq" // PostgreSQL
	"golang.org/x/crypto/bcrypt"
)

// Connect opens a Postgre database connection and initializes the schema.
// The database file and its parent directory are created automatically if they don't exist.
func Connect(dbPath string, log logger.Logger) (*pgxpool.Pool, context.Context) {
	// Open with WAL mode and foreign keys enabled for better concurrency
	config, _ := pgxpool.ParseConfig(dbPath)
	config.MaxConns = 25
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to PostgreSQL")
	}

	// SQLite performs best with limited connections

	runMigrations(pool, ctx, log)
	log.Info().Str("path", dbPath).Msg("Postgre database initialized")
	return pool, ctx
}

// runMigrations creates tables and applies schema migrations.
func runMigrations(pool *pgxpool.Pool, ctx context.Context, log logger.Logger) {
	createTables := `
	CREATE TABLE IF NOT EXISTS users (
		id SERIAL PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		phone TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user',
		created_at DATE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS bots (
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		blocked BOOLEAN DEFAULT false,
		session_file TEXT,
		payment_status TEXT DEFAULT 'free',
		created_at DATE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS prompts (
		bot_id INTEGER PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
		prompt TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS pedidos (
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		client TEXT NOT NULL,
		pedido TEXT NOT NULL,
		created_at DATE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS subscriptions (
		bot_id INTEGER PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
		tier TEXT NOT NULL DEFAULT 'free',
		msg_limit INTEGER NOT NULL DEFAULT 10,
		expires_at DATE NOT NULL
	);
	CREATE TABLE IF NOT EXISTS chat_history (
		id SERIAL PRIMARY KEY,
		bot_id INTEGER NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
		user_jid TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at DATE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS oauth_tokens (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		provider TEXT NOT NULL,
		refresh_token TEXT NOT NULL,
		created_at DATE DEFAULT CURRENT_TIMESTAMP,
		updated_at DATE DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, provider)
	);
	CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_history_bot_id_created_at ON chat_history(bot_id, created_at DESC);
	CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_bots_user_id ON bots(user_id);
	CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_pedidos_user_id_created_at ON pedidos(user_id, created_at DESC);`

	if _, err := pool.Exec(ctx, createTables); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Safe column additions for SQLite (check if column exists first)
	addColumnIfNotExists(pool, ctx, "bots", "payment_status", "TEXT DEFAULT 'free'")
	addColumnIfNotExists(pool, ctx, "subscriptions", "tier", "TEXT DEFAULT 'free'")
	addColumnIfNotExists(pool, ctx, "subscriptions", "msg_limit", "INTEGER DEFAULT 10")
}

// addColumnIfNotExists safely adds a column to a table if it doesn't already exist.
func addColumnIfNotExists(pool *pgxpool.Pool, ctx context.Context, table, column, colType string) {
	rows, err := pool.Query(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == column {
			return // Column already exists
		}
	}

	_, _ = pool.Exec(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+colType)
}

// EnsureAdmin creates the default admin user if none exists.
func EnsureAdmin(ctx context.Context, pool *pgxpool.Pool, username, email, phone, password string, log logger.Logger) {
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count); err != nil {
		log.Fatal().Err(err).Msg("Failed to query admin count")
	}
	if count == 0 {
		hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err := pool.Exec(ctx,
			`INSERT INTO users (username, email, phone, password_hash, role) VALUES ($1, $2, $3, $4, 'admin')`,
			username, email, phone, string(hashed),
		)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to create admin user")
		}
		log.Info().Str("username", username).Msg("Admin user created")
	}
}
