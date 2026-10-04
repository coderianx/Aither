package store

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB is the global database connection.
var DB *sql.DB

// ConnectDB opens a sqlite3 database connection and initializes the global DB variable.
func ConnectDB(dataSourceName string) error {
	var err error

	DB, err = sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite allows only one writer at a time. Limiting the pool to a single
	// connection avoids "database is locked" errors under concurrent writes.
	DB.SetMaxOpenConns(1)

	// Enable WAL mode to reduce SQLite concurrent write lock contention
	if _, err = DB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return fmt.Errorf("failed to set WAL mode: %w", err)
	}

	// Enable foreign key constraints
	if _, err = DB.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Wait instead of failing immediately when the database is locked
	if _, err = DB.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Create required tables
	if err = createTables(); err != nil {
		return err
	}

	return nil
}

// createTables creates the required database tables.
func createTables() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS providers (
		id            INTEGER  PRIMARY KEY AUTOINCREMENT,
		provider_name TEXT     NOT NULL,
		api_key       TEXT     NOT NULL,
		base_url      TEXT     NOT NULL,
		created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS conversations (
		id            INTEGER  PRIMARY KEY AUTOINCREMENT,
		provider_id   INTEGER  REFERENCES providers(id) ON DELETE SET NULL,
		title         TEXT     NOT NULL DEFAULT 'New Chat',
		model         TEXT     NOT NULL DEFAULT '',
		system_prompt TEXT     NOT NULL DEFAULT '',
		created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS messages (
		id              INTEGER  PRIMARY KEY AUTOINCREMENT,
		conversation_id INTEGER  NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		role            TEXT     NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
		content         TEXT     NOT NULL,
		created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_conversations_provider_id ON conversations(provider_id);
	CREATE INDEX IF NOT EXISTS idx_messages_conversation_id  ON messages(conversation_id, id);
	`

	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.Exec(schema); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit tables: %w", err)
	}

	return nil
}

// CloseDB closes the global database connection.
func CloseDB() error {
	if DB == nil {
		return nil
	}

	return DB.Close()
}
