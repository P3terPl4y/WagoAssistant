package sqlite

import (
	"App/src/pkg/logger"
	"database/sql"
	"os"
	"path/filepath"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// Connect opens a SQLite database connection and initializes the schema.
// The database file and its parent directory are created automatically if they don't exist.
func Connect(dbPath string, log logger.Logger) *sql.DB {
	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal().Err(err).Str("path", dir).Msg("Failed to create database directory")
	}

	// Open with WAL mode and foreign keys enabled for better concurrency
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(on)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open SQLite database")
	}
	if err = db.Ping(); err != nil {
		log.Fatal().Err(err).Msg("Database ping failed")
	}

	// SQLite performs best with limited connections
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	runMigrations(db, log)
	log.Info().Str("path", dbPath).Msg("SQLite database initialized")
	return db
}

// runMigrations creates tables and applies schema migrations.
func runMigrations(db *sql.DB, log logger.Logger) {
	createTables := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		email TEXT UNIQUE NOT NULL,
		phone TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'user',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS bots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		blocked INTEGER DEFAULT 0,
		session_file TEXT,
		payment_status TEXT DEFAULT 'free',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS prompts (
		bot_id INTEGER PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
		prompt TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS subscriptions (
		bot_id INTEGER PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
		tier TEXT NOT NULL DEFAULT 'free',
		msg_limit INTEGER NOT NULL DEFAULT 10,
		expires_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS chat_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		bot_id INTEGER NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
		user_jid TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS oauth_tokens (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		provider TEXT NOT NULL,
		refresh_token TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
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
			`INSERT INTO users (username, email, phone, password_hash, role) VALUES (?, ?, ?, ?, 'admin')`,
			username, email, phone, string(hashed),
		)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to create admin user")
		}
		log.Info().Str("username", username).Msg("Admin user created")
	}
}
