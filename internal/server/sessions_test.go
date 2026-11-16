package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/blob/fsblob"
	"github.com/Elven9/jarvis-core/internal/logging"
	"github.com/Elven9/jarvis-core/internal/session"
	"github.com/Elven9/jarvis-core/internal/session/sqlitestore"
	"github.com/Elven9/jarvis-core/internal/sqlite"
)

const ndjson = "application/x-ndjson"

var sessionFile = []byte("{\"type\":\"session\",\"id\":\"s1\"}\n{\"type\":\"message\"}\n")

type sessionsEnv struct {
	app      *fiber.App
	logs     *logLines
	blobs    blob.Store
	sessions session.Store
	now      time.Time
}

// newSessionsEnv serves the session routes on the real filesystem blob store and SQLite.
func newSessionsEnv(t *testing.T) *sessionsEnv {
	t.Helper()
	dir := t.TempDir()
	blobs, err := fsblob.New(filepath.Join(dir, "blobs"), filepath.Join(dir, "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(t.Context(), filepath.Join(dir, "jarvis.db"), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newSessionsEnvWith(t, blobs, sqlitestore.New(db))
}

func newSessionsEnvWith(t *testing.T, blobs blob.Store, sessions session.Store) *sessionsEnv {
	t.Helper()
	env := &sessionsEnv{logs: &logLines{}, blobs: blobs, sessions: sessions, now: time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)}
	env.app = New(logging.New(env.logs, slog.LevelDebug))
	(&Sessions{Blobs: blobs, Sessions: sessions, Now: func() time.Time { return env.now }}).Register(env.app)
	return env
}

// put sends what the pi extension sends: both segments encodeURIComponent-ed.
func (e *sessionsEnv) put(t *testing.T, machineID, sessionID, contentType string, body []byte) (*http.Response, string) {
	t.Helper()
	return e.putPath(t, "/v1/sessions/"+url.PathEscape(machineID)+"/"+url.PathEscape(sessionID), contentType, body)
}

func (e *sessionsEnv) putPath(t *testing.T, path, contentType string, body []byte) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	// Send the path exactly as given, even with malformed escapes.
	req.URL.Opaque, req.URL.RawQuery, _ = strings.Cut(path, "?")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Cf-Access-Client-Id", "id.access")
	req.Header.Set("Cf-Access-Client-Secret", "secret")
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("PUT %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(b)
}

func (e *sessionsEnv) stored(t *testing.T, machineID, sessionID string) (session.Record, []byte) {
	t.Helper()
	id, err := session.NewID(session.TypePi, machineID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := e.sessions.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("Get(%s): %v", id, err)
	}
	rc, err := e.blobs.Open(t.Context(), rec.Blob.Hash)
	if err != nil {
		t.Fatalf("Open blob: %v", err)
	}
	defer func() { _ = rc.Close() }()
	content, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return rec, content
}

func (e *sessionsEnv) assertNothingStored(t *testing.T, machineID, sessionID string) {
	t.Helper()
	id, err := session.NewID(session.TypePi, machineID, sessionID)
	if err != nil {
		return // not even a valid ID, so it cannot be stored
	}
	if _, err := e.sessions.Get(t.Context(), id); !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Get(%s): got %v, want ErrNotFound", id, err)
	}
}

func (e *sessionsEnv) putOK(t *testing.T, machineID, sessionID, contentType string, body []byte) {
	t.Helper()
	resp, b := e.put(t, machineID, sessionID, contentType, body)
	assertOK(t, resp, b)
}

func (e *sessionsEnv) putPathOK(t *testing.T, path, contentType string, body []byte) {
	t.Helper()
	resp, b := e.putPath(t, path, contentType, body)
	assertOK(t, resp, b)
}

func assertOK(t *testing.T, resp *http.Response, body string) {
	t.Helper()
	if resp.StatusCode != http.StatusOK || body != "" {
		t.Fatalf("response = %d %q, want 200 with an empty body", resp.StatusCode, body)
	}
}

// assertShortTextError checks an error answer the extension can show: the status
// and a plain-text body that fits in the 200 chars the client displays.
func assertShortTextError(t *testing.T, resp *http.Response, body string, status int) {
	t.Helper()
	if resp.StatusCode != status {
		t.Errorf("status = %d (%q), want %d", resp.StatusCode, body, status)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content-type = %q, want text/plain", ct)
	}
	if body == "" || len(body) > 200 {
		t.Errorf("body = %q, want a non-empty message of at most 200 bytes", body)
	}
}

func TestUploadStoresBodyAndAnswers200WithEmptyBody(t *testing.T) {
	env := newSessionsEnv(t)
	resp, body := env.put(t, "mac-1", "s1", ndjson, sessionFile)
	assertOK(t, resp, body)

	rec, content := env.stored(t, "mac-1", "s1")
	if !bytes.Equal(content, sessionFile) {
		t.Errorf("stored %q, want the uploaded bytes %q", content, sessionFile)
	}
	want := blob.Info{Hash: sha256.Sum256(sessionFile), Size: int64(len(sessionFile))}
	if rec.Blob != want || !rec.FirstUploadedAt.Equal(env.now) || !rec.LastUploadedAt.Equal(env.now) {
		t.Errorf("record = %+v, want blob %+v uploaded at %v", rec, want, env.now)
	}
}

func TestUploadOverwritesPreviousContent(t *testing.T) {
	env := newSessionsEnv(t)
	first := env.now
	env.putOK(t, "mac-1", "s1", ndjson, []byte("v1\n"))
	env.now = env.now.Add(time.Minute)
	v2 := []byte("v1\nv2\n")
	env.putOK(t, "mac-1", "s1", ndjson, v2)

	rec, content := env.stored(t, "mac-1", "s1")
	if !bytes.Equal(content, v2) {
		t.Errorf("stored %q, want the second upload %q", content, v2)
	}
	if !rec.FirstUploadedAt.Equal(first) || !rec.LastUploadedAt.Equal(env.now) {
		t.Errorf("record = %+v, want first upload %v and last %v", rec, first, env.now)
	}
	history, err := env.sessions.History(t.Context(), rec.ID)
	if err != nil || len(history) != 2 {
		t.Errorf("History = %+v, %v; want both uploads", history, err)
	}
}

func TestUploadDecodesPercentEncodedIDs(t *testing.T) {
	env := newSessionsEnv(t)
	sessionID := "2026-11-16 a:b%c 中文?#&+"
	resp, body := env.put(t, "Mac.home_1", sessionID, ndjson, sessionFile)
	assertOK(t, resp, body)
	if _, content := env.stored(t, "Mac.home_1", sessionID); !bytes.Equal(content, sessionFile) {
		t.Errorf("stored %q under the decoded ID, want %q", content, sessionFile)
	}
}

func TestUploadIDsAreCaseSensitive(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "S1", ndjson, []byte("upper\n"))
	env.putOK(t, "mac", "s1", ndjson, []byte("lower\n"))
	if _, content := env.stored(t, "mac", "S1"); string(content) != "upper\n" {
		t.Errorf("mac/S1 = %q, want \"upper\\n\"", content)
	}
}

func TestUploadAcceptsTextContentTypes(t *testing.T) {
	env := newSessionsEnv(t)
	for _, ct := range []string{ndjson, "application/x-ndjson; charset=utf-8", "text/plain", "text/plain; charset=utf-8", "Text/Plain", "text/x-jsonl"} {
		if resp, body := env.put(t, "mac", "s1", ct, sessionFile); resp.StatusCode != http.StatusOK || body != "" {
			t.Errorf("content-type %q: %d %q, want 200 with an empty body", ct, resp.StatusCode, body)
		}
	}
}

func TestUploadRejectsNonTextContentType(t *testing.T) {
	env := newSessionsEnv(t)
	for _, ct := range []string{"", "application/json", "application/octet-stream", "multipart/form-data; boundary=x", "text", ";;"} {
		t.Run(ct, func(t *testing.T) {
			resp, body := env.put(t, "mac", "s1", ct, sessionFile)
			assertShortTextError(t, resp, body, http.StatusUnsupportedMediaType)
			env.assertNothingStored(t, "mac", "s1")
		})
	}
}

func TestUploadRejectsEmptyBody(t *testing.T) {
	env := newSessionsEnv(t)
	resp, body := env.put(t, "mac", "s1", ndjson, nil)
	assertShortTextError(t, resp, body, http.StatusBadRequest)
	env.assertNothingStored(t, "mac", "s1")
}

func TestUploadRejectsInvalidIDs(t *testing.T) {
	env := newSessionsEnv(t)
	paths := map[string]string{
		"machineId with space":         "/v1/sessions/a%20b/s1",
		"machineId with colon":         "/v1/sessions/a:b/s1",
		"machineId over 256 bytes":     "/v1/sessions/" + strings.Repeat("a", 257) + "/s1",
		"sessionId with encoded slash": "/v1/sessions/mac/a%2Fb",
		"sessionId with NUL":           "/v1/sessions/mac/a%00b",
		"sessionId with newline":       "/v1/sessions/mac/a%0Ab",
		"sessionId invalid UTF-8":      "/v1/sessions/mac/a%FFb",
		"sessionId bad escape":         "/v1/sessions/mac/a%zzb",
		"sessionId over 256 bytes":     "/v1/sessions/mac/" + strings.Repeat("a", 257),
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			resp, body := env.putPath(t, path, ndjson, sessionFile)
			assertShortTextError(t, resp, body, http.StatusBadRequest)
		})
	}
}

func TestUploadOfDotSegmentIDNeverMatchesRoute(t *testing.T) {
	env := newSessionsEnv(t)
	// The router removes "." and ".." segments (also encoded) before matching.
	for _, path := range []string{"/v1/sessions/mac/%2E", "/v1/sessions/mac/%2E%2E", "/v1/sessions/./s1"} {
		if resp, _ := env.putPath(t, path, ndjson, sessionFile); resp.StatusCode != http.StatusNotFound {
			t.Errorf("PUT %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestUploadTypeDefaultsToPiAndRejectsOthers(t *testing.T) {
	env := newSessionsEnv(t)
	env.putPathOK(t, "/v1/sessions/mac/s1?type=pi", ndjson, sessionFile)
	for _, query := range []string{"?type=claude", "?type=", "?type=PI", "?type=pi&type=pi"} {
		t.Run(query, func(t *testing.T) {
			resp, body := env.putPath(t, "/v1/sessions/mac/s2"+query, ndjson, sessionFile)
			assertShortTextError(t, resp, body, http.StatusBadRequest)
			env.assertNothingStored(t, "mac", "s2")
		})
	}
}

func TestUploadStreamsBodyLargerThanBodyLimit(t *testing.T) {
	env := newSessionsEnv(t)
	content := make([]byte, 10<<20) // over Fiber's 4 MB default body limit
	_, _ = rand.NewChaCha8([32]byte{7}).Read(content)
	env.putOK(t, "mac", "big", ndjson, content)
	if _, stored := env.stored(t, "mac", "big"); !bytes.Equal(stored, content) {
		t.Error("stored content differs from the 10 MB upload")
	}
}

func TestUploadIsOnlyServedUnderV1(t *testing.T) {
	env := newSessionsEnv(t)
	for _, path := range []string{"/sessions/mac/s1", "/V1/sessions/mac/s1", "/v1/sessions/mac/s1/"} {
		if resp, _ := env.putPath(t, path, ndjson, sessionFile); resp.StatusCode != http.StatusNotFound {
			t.Errorf("PUT %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestUploadStoreFailureAnswers500AndRecordsNothing(t *testing.T) {
	dir := t.TempDir()
	db, err := sqlite.Open(t.Context(), filepath.Join(dir, "jarvis.db"), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	env := newSessionsEnvWith(t, failingBlobs{}, sqlitestore.New(db))

	resp, body := env.put(t, "mac", "s1", ndjson, sessionFile)
	assertShortTextError(t, resp, body, http.StatusInternalServerError)
	if strings.Contains(body, "disk full") {
		t.Errorf("body %q leaks the internal error", body)
	}
	env.assertNothingStored(t, "mac", "s1")
}

func TestUploadAccessLogHasSessionIDFields(t *testing.T) {
	env := newSessionsEnv(t)
	env.put(t, "mac-1", "s 1", ndjson, sessionFile)
	r := onlyRecord(t, env.logs)
	if r["machine_id"] != "mac-1" || r["session_id"] != "s 1" || r["status"] != 200.0 {
		t.Errorf("access log = %v, want machine_id, session_id and status 200", r)
	}
}

type failingBlobs struct{}

func (failingBlobs) Put(context.Context, io.Reader) (blob.Info, error) {
	return blob.Info{}, errors.New("disk full")
}

func (failingBlobs) Open(context.Context, blob.Hash) (io.ReadCloser, error) {
	return nil, blob.ErrNotFound
}
