package sqlite

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestOpenCreatesDatabaseWithSchema(t *testing.T) {
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "sub", "jarvis.db"), discard)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	for _, table := range []string{"sessions", "uploads"} {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE type = 'table' AND name = ?", table).Scan(&n); err != nil || n != 1 {
			t.Errorf("table %s: count %d, err %v; want it created", table, n, err)
		}
	}
}

func TestOpenEnablesWALAndForeignKeys(t *testing.T) {
	db, err := Open(t.Context(), filepath.Join(t.TempDir(), "jarvis.db"), discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var mode string
	var fk int
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v; want wal", mode, err)
	}
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, %v; want 1", fk, err)
	}
}

func TestOpenIsIdempotentAndKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jarvis.db")
	db, err := Open(t.Context(), path, discard)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO sessions VALUES ('pi', 'm', 's', ?, 1, 0, 0)`, strings.Repeat("a", 64))
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	db, err = Open(t.Context(), path, discard)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM sessions").Scan(&n); err != nil || n != 1 {
		t.Errorf("sessions after reopening = %d, %v; want 1", n, err)
	}
}

func TestOpenAppliesNewMigrationsToExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jarvis.db")
	v1 := fstest.MapFS{"00001_a.sql": {Data: []byte("-- +goose Up\nCREATE TABLE a (x INTEGER);\n")}}
	v2 := fstest.MapFS{
		"00001_a.sql": v1["00001_a.sql"],
		"00002_b.sql": {Data: []byte("-- +goose Up\nCREATE TABLE b (x INTEGER);\n")},
	}
	db, err := open(t.Context(), path, discard, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	db, err = open(t.Context(), path, discard, v2)
	if err != nil {
		t.Fatalf("Open with a new migration: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec("INSERT INTO b VALUES (1)"); err != nil {
		t.Errorf("new migration not applied: %v", err)
	}
}

func TestOpenRefusesDatabaseNewerThanBinary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jarvis.db")
	v2 := fstest.MapFS{
		"00001_a.sql": {Data: []byte("-- +goose Up\nCREATE TABLE a (x INTEGER);\n")},
		"00002_b.sql": {Data: []byte("-- +goose Up\nCREATE TABLE b (x INTEGER);\n")},
	}
	db, err := open(t.Context(), path, discard, v2)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	v1 := fstest.MapFS{"00001_a.sql": v2["00001_a.sql"]}
	if db, err := open(t.Context(), path, discard, v1); err == nil {
		_ = db.Close()
		t.Fatal("Open succeeded on a database newer than the binary")
	} else if !strings.Contains(err.Error(), "newer") {
		t.Errorf("Open: %v, want an error saying the schema is newer", err)
	}
}

func TestOpenRollsBackFailingMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jarvis.db")
	bad := fstest.MapFS{"00001_a.sql": {Data: []byte("-- +goose Up\nCREATE TABLE a (x INTEGER);\nNOT SQL;\n")}}
	if db, err := open(t.Context(), path, discard, bad); err == nil {
		_ = db.Close()
		t.Fatal("Open succeeded with a failing migration")
	}
	good := fstest.MapFS{"00001_a.sql": {Data: []byte("-- +goose Up\nCREATE TABLE a (x INTEGER);\n")}}
	db, err := open(t.Context(), path, discard, good)
	if err != nil {
		t.Fatalf("Open after a rolled-back migration: %v", err)
	}
	_ = db.Close()
}
