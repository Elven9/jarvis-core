package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/Elven9/jarvis-core/internal/logging"
)

type logLines struct{ bytes.Buffer }

func (l *logLines) records(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(l.String()) {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		out = append(out, r)
	}
	return out
}

func newTestApp(t *testing.T) (*fiber.App, *logLines) {
	t.Helper()
	logs := &logLines{}
	app := New(logging.New(logs, slog.LevelDebug))
	app.Get("/ok", func(c fiber.Ctx) error { return c.SendString("hello") })
	app.Get("/teapot", func(fiber.Ctx) error { return fiber.NewError(fiber.StatusTeapot, "short and stout") })
	app.Get("/internal", func(fiber.Ctx) error { return errors.New("db password is hunter2") })
	app.Get("/panic", func(fiber.Ctx) error { panic("boom") })
	app.Put("/echo", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	return app, logs
}

func do(t *testing.T, app *fiber.App, req *http.Request) (*http.Response, string) {
	t.Helper()
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
}

func onlyRecord(t *testing.T, logs *logLines) map[string]any {
	t.Helper()
	records := logs.records(t)
	if len(records) != 1 {
		t.Fatalf("got %d log records, want 1: %v", len(records), records)
	}
	return records[0]
}

func TestAccessLogRecordsEachRequest(t *testing.T) {
	app, logs := newTestApp(t)
	resp, _ := do(t, app, httptest.NewRequest(http.MethodGet, "/ok?x=1", nil))
	r := onlyRecord(t, logs)
	if r["msg"] != "request" || r["method"] != "GET" || r["path"] != "/ok" || r["status"] != 200.0 || r["size"] != 5.0 {
		t.Errorf("record = %v, want GET /ok 200 size 5", r)
	}
	if _, ok := r["latency"]; !ok {
		t.Errorf("record = %v, want a latency", r)
	}
	if id := resp.Header.Get(fiber.HeaderXRequestID); id == "" || r["request_id"] != id {
		t.Errorf("request_id = %v, want the response's X-Request-ID %q", r["request_id"], id)
	}
}

func TestAccessLogRecordsFinalStatusAndErrorOfFailedRequest(t *testing.T) {
	app, logs := newTestApp(t)
	resp, body := do(t, app, httptest.NewRequest(http.MethodGet, "/teapot", nil))
	if resp.StatusCode != fiber.StatusTeapot || body != "short and stout" {
		t.Errorf("response = %d %q, want 418 \"short and stout\"", resp.StatusCode, body)
	}
	r := onlyRecord(t, logs)
	if r["status"] != 418.0 || r["error"] != "short and stout" {
		t.Errorf("record = %v, want status 418 and the error", r)
	}
}

func TestUnexpectedErrorsReturnBare500(t *testing.T) {
	app, logs := newTestApp(t)
	resp, body := do(t, app, httptest.NewRequest(http.MethodGet, "/internal", nil))
	if resp.StatusCode != fiber.StatusInternalServerError || body != "Internal Server Error" {
		t.Errorf("response = %d %q, want a bare 500", resp.StatusCode, body)
	}
	if r := onlyRecord(t, logs); r["status"] != 500.0 || r["error"] != "db password is hunter2" {
		t.Errorf("record = %v, want status 500 and the internal error", r)
	}
}

func TestPanicsAreRecoveredAs500(t *testing.T) {
	app, logs := newTestApp(t)
	resp, body := do(t, app, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if resp.StatusCode != fiber.StatusInternalServerError || body != "Internal Server Error" {
		t.Errorf("response = %d %q, want a bare 500", resp.StatusCode, body)
	}
	if r := onlyRecord(t, logs); r["status"] != 500.0 || !strings.Contains(r["error"].(string), "boom") {
		t.Errorf("record = %v, want status 500 and the panic", r)
	}
}

func TestAccessLogNeverContainsBodyOrHeaders(t *testing.T) {
	app, logs := newTestApp(t)
	req := httptest.NewRequest(http.MethodPut, "/echo", strings.NewReader("SECRET-BODY"))
	req.Header.Set("Cf-Access-Client-Secret", "SECRET-HEADER")
	do(t, app, req)
	out := logs.String()
	if strings.Contains(out, "SECRET-BODY") || strings.Contains(out, "SECRET-HEADER") {
		t.Errorf("access log leaks the request: %s", out)
	}
}

func TestRoutesAreCaseSensitiveAndStrict(t *testing.T) {
	app, _ := newTestApp(t)
	for _, path := range []string{"/OK", "/ok/"} {
		if resp, _ := do(t, app, httptest.NewRequest(http.MethodGet, path, nil)); resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}
