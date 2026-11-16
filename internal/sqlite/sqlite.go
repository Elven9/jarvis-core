// Package sqlite opens the server's SQLite database and brings its schema up to date.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // registers the "sqlite3" driver
	"github.com/pressly/goose/v3"

	"github.com/Elven9/jarvis-core/migrations"
)

// dsnOptions: WAL, foreign keys, a 5 s busy timeout, and BEGIN IMMEDIATE so a
// transaction that reads then writes does not fail with "database is locked".
const dsnOptions = "?_journal_mode=WAL&_foreign_keys=1&_busy_timeout=5000&_txlock=immediate"

// Open opens (creating if needed) the database at path and applies the embedded migrations.
// It refuses a database whose schema is newer than this binary knows.
func Open(ctx context.Context, path string, logger *slog.Logger) (*sql.DB, error) {
	return open(ctx, path, logger, migrations.FS)
}

func open(ctx context.Context, path string, logger *slog.Logger, fsys fs.FS) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite3", "file:"+path+dsnOptions)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	if err := migrate(ctx, db, logger, fsys); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database %s: %w", path, err)
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB, logger *slog.Logger, fsys fs.FS) error {
	p, err := goose.NewProvider(goose.DialectSQLite3, db, fsys, goose.WithSlog(logger), goose.WithVerbose(true))
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	current, err := p.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	sources := p.ListSources()
	var latest int64
	if len(sources) > 0 {
		latest = sources[len(sources)-1].Version
	}
	if current > latest {
		return fmt.Errorf("schema version %d is newer than this binary supports (%d)", current, latest)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	logger.InfoContext(ctx, "database schema ready", "schema_version", latest, "previous_version", current)
	return nil
}
