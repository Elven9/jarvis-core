package sqlitestore

import (
	"crypto/sha256"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/session"
	"github.com/Elven9/jarvis-core/internal/session/sessiontest"
	"github.com/Elven9/jarvis-core/internal/sqlite"
)

func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sqlite.Open(t.Context(), path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestStorePassesContract(t *testing.T) {
	sessiontest.Run(t, func(t *testing.T) session.Store {
		return New(openDB(t, filepath.Join(t.TempDir(), "jarvis.db")))
	})
}

func TestRecordsSurviveReopening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jarvis.db")
	id, err := session.NewID(session.TypePi, "mac", "s1")
	if err != nil {
		t.Fatal(err)
	}
	u := session.Upload{
		ID:         id,
		Blob:       blob.Info{Hash: sha256.Sum256([]byte("v1")), Size: 2},
		UploadedAt: time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC),
	}
	db := openDB(t, path)
	if err := New(db).RecordUpload(t.Context(), u); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	got, err := New(openDB(t, path)).Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get after reopening: %v", err)
	}
	if got.Blob != u.Blob || !got.LastUploadedAt.Equal(u.UploadedAt) {
		t.Errorf("Get = %+v, want the recorded upload %+v", got, u)
	}
}
