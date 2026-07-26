package postgre

import (
	"App/src/pkg/logger"
	"database/sql"

	_ "github.com/lib/pq" // PostgreSQL

	"golang.org/x/crypto/bcrypt"
)

// Connect opens a Postgre database connection and initializes the schema.
// The database file and its parent directory are created automatically if they don't exist.
func Connect(dbPath string, log logger.Logger) *sql.DB {
	// Open with WAL mode and foreign keys enabled for better concurrency
	db, err := sql.Open("postgres", dbPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open Postgre database")
	}
	if err = db.Ping(); err != nil {
		log.Fatal().Err(err).Msg("Database ping failed")
	}

	// SQLite performs best with limited connections
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	runMigrations(db, log)
	log.Info().Str("path", dbPath).Msg("Postgre database initialized")
	return db
}

// runMigrations creates tables and applies schema migrations.
func runMigrations(db *sql.DB, log logger.Logger) {
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
		blocked INTEGER DEFAULT 0,
		session_file TEXT,
		payment_status TEXT DEFAULT 'free',
		created_at DATE DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS prompts (
		bot_id INTEGER PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
		prompt TEXT NOT NULL
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
	);`

	if _, err := db.Exec(createTables); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Safe column additions for SQLite (check if column exists first)
	addColumnIfNotExists(db, "bots", "payment_status", "TEXT DEFAULT 'free'")
	addColumnIfNotExists(db, "subscriptions", "tier", "TEXT DEFAULT 'free'")
	addColumnIfNotExists(db, "subscriptions", "msg_limit", "INTEGER DEFAULT 10")
}

// addColumnIfNotExists safely adds a column to a table if it doesn't already exist.
func addColumnIfNotExists(db *sql.DB, table, column, colType string) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
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

	_, _ = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + colType)
}

// EnsureAdmin creates the default admin user if none exists.
func EnsureAdmin(db *sql.DB, username, email, phone, password string, log logger.Logger) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&count); err != nil {
		log.Fatal().Err(err).Msg("Failed to query admin count")
	}
	if count == 0 {
		hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		_, err := db.Exec(
			`INSERT INTO users (username, email, phone, password_hash, role) VALUES ($1, $2, $3, $4, 'admin')`,
			username, email, phone, string(hashed),
		)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to create admin user")
		}
		log.Info().Str("username", username).Msg("Admin user created")
	}
}
