package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// Database wraps *sql.DB and dialect information.
type Database struct {
	*sql.DB
	IsPostgres bool
}

// Connect opens a database connection based on the connection string
// and returns a Database wrapper.
func Connect(databaseURL string) (*Database, error) {
	var driverName, dsn string
	var isPostgres bool

	trimmed := strings.TrimSpace(databaseURL)
	if strings.HasPrefix(trimmed, "postgres://") || strings.HasPrefix(trimmed, "postgresql://") {
		driverName = "postgres"
		dsn = trimmed
		isPostgres = true
	} else {
		driverName = "sqlite"
		isPostgres = false
		// Clean SQLite path
		dsn = strings.TrimPrefix(trimmed, "sqlite:///")
		dsn = strings.TrimPrefix(dsn, "sqlite://")
		dsn = strings.TrimPrefix(dsn, "sqlite:")
		if dsn == "" {
			dsn = "./url_shortener.db"
		}

		// Ensure directory exists if path contains folders
		if dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") {
			dir := filepath.Dir(dsn)
			if dir != "." && dir != "" {
				_ = os.MkdirAll(dir, 0755)
			}
		}
	}

	sqlDB, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database (%s): %w", driverName, err)
	}

	// Connection pool tuning
	if !isPostgres && dsn == ":memory:" {
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxOpenConns(25)
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetConnMaxLifetime(5 * time.Minute)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &Database{
		DB:         sqlDB,
		IsPostgres: isPostgres,
	}

	if err := AutoMigrate(d); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to auto-migrate database schema: %w", err)
	}

	return d, nil
}

// AutoMigrate creates the required tables and indexes if they do not exist.
func AutoMigrate(d *Database) error {
	var createTableSQL string
	if d.IsPostgres {
		createTableSQL = `
CREATE TABLE IF NOT EXISTS url_mappings (
    id SERIAL PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    long_url TEXT NOT NULL,
    clicks INTEGER DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_short_code ON url_mappings(short_code);
`
	} else {
		createTableSQL = `
CREATE TABLE IF NOT EXISTS url_mappings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    long_url TEXT NOT NULL,
    clicks INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_short_code ON url_mappings(short_code);
`
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := d.ExecContext(ctx, createTableSQL); err != nil {
		return fmt.Errorf("executing migration query failed: %w", err)
	}
	return nil
}
