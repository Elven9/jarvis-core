package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/session"
)

func (e *sessionsEnv) get(t *testing.T, machineID, sessionID string) (*http.Response, []byte) {
	t.Helper()
	return e.getPath(t, "/v1/sessions/"+url.PathEscape(machineID)+"/"+url.PathEscape(sessionID))
}

func (e *sessionsEnv) getPath(t *testing.T, path string) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Opaque, req.URL.RawQuery, _ = strings.Cut(path, "?")
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, body
}

func assertSession(t *testing.T, resp *http.Response, body, want []byte) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("content-type = %q, want application/x-ndjson", ct)
	}
	if cl := resp.Header.Get("Content-Length"); cl != strconv.Itoa(len(want)) {
		t.Errorf("content-length = %q, want %d", cl, len(want))
	}
	if !bytes.Equal(body, want) {
		t.Errorf("body (%d bytes) differs from the upload (%d bytes)", len(body), len(want))
	}
}

// assertProblem checks an RFC 9457 problem details answer.
func assertProblem(t *testing.T, resp *http.Response, body []byte, status int) map[string]any {
	t.Helper()
	if resp.StatusCode != status {
		t.Errorf("status = %d (%s), want %d", resp.StatusCode, body, status)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("content-type = %q, want application/problem+json", ct)
	}
	var p map[string]any
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("body %q is not JSON: %v", body, err)
	}
	if p["type"] != "about:blank" || p["title"] != http.StatusText(status) || p["status"] != float64(status) {
		t.Errorf("problem = %v, want type about:blank, title %q, status %d", p, http.StatusText(status), status)
	}
	return p
}

func TestReadReturnsUploadedBytes(t *testing.T) {
	env := newSessionsEnv(t)
	sessionFile := piSession(t, "s1")
	env.putOK(t, "mac-1", "s1", ndjson, sessionFile)
	resp, body := env.get(t, "mac-1", "s1")
	assertSession(t, resp, body, sessionFile)
	if etag := resp.Header.Get("ETag"); etag != "" {
		t.Errorf("ETag = %q, want none", etag)
	}
}

func TestReadReturnsLatestUpload(t *testing.T) {
	env := newSessionsEnv(t)
	v2 := piSession(t, "s1")
	env.putOK(t, "mac-1", "s1", ndjson, firstLines(v2, 4))
	env.putOK(t, "mac-1", "s1", ndjson, v2)
	resp, body := env.get(t, "mac-1", "s1")
	assertSession(t, resp, body, v2)
}

func TestReadStreamsLargeSession(t *testing.T) {
	env := newSessionsEnv(t)
	content := bigSession(t, "big", 10<<20)
	env.putOK(t, "mac", "big", ndjson, content)
	resp, body := env.get(t, "mac", "big")
	assertSession(t, resp, body, content)
}

func TestReadDecodesPercentEncodedIDs(t *testing.T) {
	env := newSessionsEnv(t)
	sessionID := "2026-11-16 a:b%c 中文?#&+"
	sessionFile := piSession(t, sessionID)
	env.putOK(t, "Mac.home_1", sessionID, ndjson, sessionFile)
	resp, body := env.get(t, "Mac.home_1", sessionID)
	assertSession(t, resp, body, sessionFile)
}

func TestReadIDsAreCaseSensitive(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "s1", ndjson, piSession(t, "s1"))
	resp, body := env.get(t, "mac", "S1")
	assertProblem(t, resp, body, http.StatusNotFound)
}

func TestReadOfUnknownSessionIsNotFoundProblem(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "other", ndjson, piSession(t, "other"))
	resp, body := env.get(t, "mac", "s1")
	p := assertProblem(t, resp, body, http.StatusNotFound)
	if detail, _ := p["detail"].(string); !strings.Contains(detail, "pi:mac:s1") {
		t.Errorf("detail = %q, want it to name the session", detail)
	}
}

func TestReadRejectsInvalidIDsWithProblem(t *testing.T) {
	env := newSessionsEnv(t)
	paths := []string{
		"/v1/sessions/a%20b/s1",
		"/v1/sessions/" + strings.Repeat("a", 257) + "/s1",
		"/v1/sessions/mac/a%2Fb",
		"/v1/sessions/mac/a%00b",
		"/v1/sessions/mac/a%FFb",
		"/v1/sessions/mac/a%zzb",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			resp, body := env.getPath(t, path)
			if p := assertProblem(t, resp, body, http.StatusBadRequest); p["detail"] == nil {
				t.Errorf("problem = %v, want a detail saying what is wrong", p)
			}
		})
	}
}

func TestReadTypeDefaultsToPiAndRejectsOthers(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "s1", ndjson, piSession(t, "s1"))
	resp, body := env.getPath(t, "/v1/sessions/mac/s1?type=pi")
	assertSession(t, resp, body, piSession(t, "s1"))
	for _, query := range []string{"?type=claude", "?type=", "?type=PI", "?type=pi&type=pi"} {
		resp, body := env.getPath(t, "/v1/sessions/mac/s1"+query)
		assertProblem(t, resp, body, http.StatusBadRequest)
	}
}

func TestReadStoreFailureIsInternalProblemWithoutDetails(t *testing.T) {
	env := newSessionsEnvWith(t, failingBlobs{}, &oneSession{}, nil)
	resp, body := env.get(t, "mac", "s1")
	p := assertProblem(t, resp, body, http.StatusInternalServerError)
	if _, ok := p["detail"]; ok {
		t.Errorf("problem = %v, want no detail for an internal error", p)
	}
	if r := onlyRecord(t, env.logs); r["status"] != 500.0 || r["error"] == nil {
		t.Errorf("access log = %v, want status 500 and the internal error", r)
	}
}

func TestReadAccessLogHasSessionIDFields(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac-1", "s 1", ndjson, piSession(t, "s 1"))
	env.logs.Reset()
	env.get(t, "mac-1", "s 1")
	r := onlyRecord(t, env.logs)
	if r["method"] != "GET" || r["machine_id"] != "mac-1" || r["session_id"] != "s 1" || r["status"] != 200.0 || r["size"] != float64(len(piSession(t, "s 1"))) {
		t.Errorf("access log = %v, want GET 200 with the ID fields and size", r)
	}
}

// oneSession is a session.Store that knows every session, pointing at a blob that does not exist.
type oneSession struct{}

func (*oneSession) RecordUpload(context.Context, session.Upload) error { return nil }

func (*oneSession) Get(_ context.Context, id session.ID) (session.Record, error) {
	return session.Record{ID: id, Blob: blob.Info{Hash: sha256.Sum256([]byte("gone")), Size: 4}}, nil
}

func (*oneSession) History(context.Context, session.ID) ([]session.Upload, error) { return nil, nil }
