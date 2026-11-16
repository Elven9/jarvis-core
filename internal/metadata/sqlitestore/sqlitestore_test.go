package sqlitestore

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/metadata/metadatatest"
	"github.com/Elven9/jarvis-core/internal/session"
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
	metadatatest.Run(t, func(t *testing.T) metadata.Store {
		return New(openDB(t, filepath.Join(t.TempDir(), "jarvis.db")))
	})
}

func TestMetadataSurvivesReopening(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jarvis.db")
	id, err := session.NewID(session.TypePi, "mac", "s1")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	m := metadata.Meta{
		ID:            id,
		Metadata:      sessionparser.Metadata{SessionID: "s1", CWD: "/tmp", StartedAt: at, FormatVersion: 3},
		Metrics:       &sessionparser.Metrics{Turns: 3, EntryID: "e1", At: at},
		ParserVersion: "v0.1.0",
	}
	db := openDB(t, path)
	if err := New(db).Replace(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	got, err := New(openDB(t, path)).Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get after reopening: %v", err)
	}
	if got.ParserVersion != m.ParserVersion || got.Metrics == nil || got.Metrics.Turns != 3 {
		t.Errorf("Get = %+v, want the replaced metadata %+v", got, m)
	}
}

func TestReplaceLeavesNoRowsOfThePreviousMeta(t *testing.T) {
	db := openDB(t, filepath.Join(t.TempDir(), "jarvis.db"))
	s := New(db)
	id, err := session.NewID(session.TypePi, "mac", "s1")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	full := metadata.Meta{
		ID: id,
		Metadata: sessionparser.Metadata{
			SessionID: "s1", CWD: "/tmp", StartedAt: at, FormatVersion: 3,
			Tags:   &sessionparser.Tags{Tags: []string{"a", "b"}, Source: "flag", At: at},
			Models: []sessionparser.ModelChange{{Provider: "p", ModelID: "m", At: at}},
			Errors: []sessionparser.AgentError{{Message: "x", At: at}},
		},
		Metrics: &sessionparser.Metrics{
			Tools:       map[string]sessionparser.ToolMetrics{"bash": {Calls: 1}},
			CacheMisses: []sessionparser.CacheMiss{{Turn: 1}},
			EntryID:     "e1", At: at,
		},
		ParserVersion: "v0.1.0",
	}
	minimal := metadata.Meta{ID: id, Metadata: sessionparser.Metadata{SessionID: "s1", CWD: "/tmp", StartedAt: at, FormatVersion: 3}, ParserVersion: "v0.1.0"}
	for _, m := range []metadata.Meta{full, minimal} {
		if err := s.Replace(t.Context(), m); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"session_tags", "session_models", "session_errors", "session_metrics", "session_tool_metrics", "session_cache_misses"} {
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s has %d rows (%v), want 0 after replacing with metadata without them", table, n, err)
		}
	}
}
