package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// startRun runs the server in the background and returns its log records as they appear.
func startRun(t *testing.T, ctx context.Context, environ []string) (<-chan map[string]any, <-chan error) {
	t.Helper()
	pr, pw := io.Pipe()
	records := make(chan map[string]any, 100)
	done := make(chan error, 1)
	go func() {
		err := run(ctx, environ, pw)
		_ = pw.Close()
		done <- err
	}()
	go func() {
		defer close(records)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			var r map[string]any
			if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
				t.Errorf("stdout line is not JSON: %q", sc.Text())
				continue
			}
			records <- r
		}
	}()
	return records, done
}

func waitFor(t *testing.T, records <-chan map[string]any, msg string) map[string]any {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case r, ok := <-records:
			if !ok {
				t.Fatalf("log ended before %q", msg)
			}
			if r["msg"] == msg {
				return r
			}
		case <-timeout:
			t.Fatalf("no %q log record within 10s", msg)
		}
	}
}

func TestRunServesUntilContextIsDoneThenStops(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	records, done := startRun(t, ctx, []string{"JARVIS_ADDR=127.0.0.1:0", "JARVIS_SHUTDOWN_TIMEOUT=2s", "JARVIS_DATA_DIR=" + t.TempDir()})

	starting := waitFor(t, records, "starting")
	if cfg, ok := starting["config"].(map[string]any); !ok || cfg["addr"] != "127.0.0.1:0" || cfg["shutdown_timeout"] != "2s" {
		t.Errorf("starting record = %v, want the effective config", starting)
	}
	addr := waitFor(t, records, "listening")["addr"].(string)

	resp, err := http.Get("http://" + addr + "/v1/nothing-here")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET unknown route = %d, want 404", resp.StatusCode)
	}
	if r := waitFor(t, records, "request"); r["status"] != 404.0 {
		t.Errorf("access log = %v, want status 404", r)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run returned %v after shutdown, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not return after the context was canceled")
	}
	waitFor(t, records, "stopped")
}

func TestRunMigratesDatabaseBeforeListening(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dataDir := filepath.Join(t.TempDir(), "data")
	records, done := startRun(t, ctx, []string{"JARVIS_ADDR=127.0.0.1:0", "JARVIS_DATA_DIR=" + dataDir})
	waitFor(t, records, "listening")

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dataDir, "jarvis.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_schema WHERE name IN ('sessions', 'uploads')").Scan(&n); err != nil || n != 2 {
		t.Errorf("schema tables = %d, %v; want sessions and uploads", n, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestRunRefusesToStartWhenDatabaseCannotOpen(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	records, done := startRun(t, t.Context(), []string{"JARVIS_ADDR=127.0.0.1:0", "JARVIS_DATA_DIR=" + notADir})
	if r := waitFor(t, records, "cannot open database"); r["level"] != "ERROR" {
		t.Errorf("record = %v, want level ERROR", r)
	}
	if err := <-done; err == nil {
		t.Error("run returned nil, want the database error")
	}
}

func TestRunRefusesToStartWithInvalidConfig(t *testing.T) {
	records, done := startRun(t, t.Context(), []string{"JARVIS_LOG_LEVEL=loud"})
	r := waitFor(t, records, "invalid configuration")
	if r["level"] != "ERROR" {
		t.Errorf("record = %v, want level ERROR", r)
	}
	if err := <-done; err == nil {
		t.Error("run returned nil, want the config error")
	}
}
