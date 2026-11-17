package server

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

// listRows gets the list route and decodes its 200 JSON body.
func (e *sessionsEnv) listRows(t *testing.T) []map[string]any {
	t.Helper()
	resp, body := e.getPath(t, "/v1/sessions")
	v := decodeJSON(t, resp, body)
	raw, ok := v["sessions"].([]any)
	if !ok || len(v) != 1 {
		t.Fatalf("body = %s, want an object with only a sessions list", body)
	}
	rows := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		rows = append(rows, r.(map[string]any))
	}
	return rows
}

func rowIDs(rows []map[string]any) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r["machineId"].(string)+"/"+r["sessionId"].(string))
	}
	return out
}

func TestListAnswersEveryRowFieldInItsJSONShape(t *testing.T) {
	at := time.Date(2026, 11, 16, 9, 0, 0, 123_000_000, time.UTC)
	p := func(s string) *string { return &s }
	id := func(m, s string) session.ID { sid, _ := session.NewID(session.TypePi, m, s); return sid }
	env := newSessionsEnvWith(t, failingBlobs{}, &oneSession{}, listedMeta{
		{ID: id("mac", "s1"), UploadedAt: at, Name: p("review"), AgentVersion: p("0.12.0"), Tags: []string{"b", "a"},
			Scenario: p("code-writer"), Project: p("jarvis"),
			Metrics: &metadata.MetricsSummary{Tokens: sessionparser.Tokens{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4}, CostUSD: 0.5, Turns: 6}},
		{ID: id("mac", "s2"), UploadedAt: at, Tags: []string{}},
		{ID: id("mac", "s3"), UploadedAt: at},
	})
	resp, body := env.getPath(t, "/v1/sessions")
	got := decodeJSON(t, resp, body)

	var want map[string]any
	if err := json.Unmarshal([]byte(`{"sessions": [
		{"machineId": "mac", "sessionId": "s1", "uploadedAt": "2026-11-16T09:00:00.123Z",
		 "name": "review", "tags": ["b", "a"], "agentVersion": "0.12.0", "scenario": "code-writer", "project": "jarvis",
		 "metrics": {"tokens": {"input": 1, "output": 2, "cacheRead": 3, "cacheWrite": 4}, "costUsd": 0.5, "turns": 6}},
		{"machineId": "mac", "sessionId": "s2", "uploadedAt": "2026-11-16T09:00:00.123Z",
		 "name": null, "tags": [], "agentVersion": null, "scenario": null, "project": null, "metrics": null},
		{"machineId": "mac", "sessionId": "s3", "uploadedAt": "2026-11-16T09:00:00.123Z",
		 "name": null, "tags": null, "agentVersion": null, "scenario": null, "project": null, "metrics": null}
	]}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		gj, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("body =\n%s\nwant the documented shape", gj)
	}
}

func TestListWithoutSessionsIsEmptyList(t *testing.T) {
	env := newSessionsEnv(t)
	if rows := env.listRows(t); len(rows) != 0 {
		t.Errorf("rows = %v, want []", rows)
	}
}

func TestListShowsEveryUploadedSessionNewestFirst(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "a", ndjson, piSession(t, "a"))
	env.now = env.now.Add(time.Minute)
	tags := golden(t, "tags")
	tagsID := headerID(t, tags)
	env.putOK(t, "mac", tagsID, ndjson, tags)
	env.now = env.now.Add(time.Minute)
	env.putOK(t, "linux", "c", ndjson, piSession(t, "c"))

	rows := env.listRows(t)
	if got, want := rowIDs(rows), []string{"linux/c", "mac/" + tagsID, "mac/a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	r := rows[1]
	if r["uploadedAt"] != env.now.Add(-time.Minute).Format("2006-01-02T15:04:05Z07:00") ||
		!reflect.DeepEqual(r["tags"], []any{"helloworld"}) || r["metrics"] == nil {
		t.Errorf("row = %v, want the upload time, tags [helloworld] and metrics", r)
	}
	stored, err := env.meta(t, "mac", tagsID)
	if err != nil {
		t.Fatal(err)
	}
	m := r["metrics"].(map[string]any)
	if m["turns"] != float64(stored.Metrics.Turns) || m["costUsd"] != stored.Metrics.CostUSD ||
		m["tokens"].(map[string]any)["input"] != float64(stored.Metrics.Tokens.Input) {
		t.Errorf("row metrics = %v, want the stored totals %+v", m, stored.Metrics)
	}
	if named := rows[0]; named["name"] != "subagent a2 (demo)" {
		t.Errorf("row = %v, want the session name", named)
	}
}

func TestListOrdersSameUploadTimeBySessionID(t *testing.T) {
	env := newSessionsEnv(t)
	for _, sid := range []string{"b", "a", "B"} {
		env.putOK(t, "mac", sid, ndjson, piSession(t, sid))
	}
	if got, want := rowIDs(env.listRows(t)), []string{"mac/B", "mac/a", "mac/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestListMovesReuploadedSessionToTheTopOnce(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "a", ndjson, piSession(t, "a"))
	env.now = env.now.Add(time.Minute)
	env.putOK(t, "mac", "b", ndjson, piSession(t, "b"))
	env.now = env.now.Add(time.Minute)
	env.putOK(t, "mac", "a", ndjson, piSession(t, "a"))
	if got, want := rowIDs(env.listRows(t)), []string{"mac/a", "mac/b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestListLeavesOutRejectedUploads(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "a", ndjson, piSession(t, "a"))
	resp, body := env.put(t, "mac", "bad", ndjson, []byte("hello\n"))
	assertShortTextError(t, resp, body, http.StatusUnprocessableEntity)
	if got := rowIDs(env.listRows(t)); !reflect.DeepEqual(got, []string{"mac/a"}) {
		t.Errorf("rows = %v, want only mac/a", got)
	}
}

func TestListStoreFailureIsInternalProblemWithoutDetails(t *testing.T) {
	env := newSessionsEnvWith(t, failingBlobs{}, &oneSession{}, brokenMeta{})
	resp, body := env.getPath(t, "/v1/sessions")
	p := assertProblem(t, resp, body, http.StatusInternalServerError)
	if _, ok := p["detail"]; ok {
		t.Errorf("problem = %v, want no detail for an internal error", p)
	}
}

func TestListIsOnlyServedAtV1Sessions(t *testing.T) {
	env := newSessionsEnv(t)
	for _, path := range []string{"/v1/sessions/", "/V1/sessions", "/sessions"} {
		if resp, _ := env.getPath(t, path); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

// listedMeta is a metadata.Store whose List answers its rows.
type listedMeta []metadata.Summary

func (listedMeta) Replace(context.Context, metadata.Meta) error { return nil }

func (listedMeta) Get(context.Context, session.ID) (metadata.Meta, error) {
	return metadata.Meta{}, metadata.ErrNotFound
}

func (l listedMeta) List(context.Context) ([]metadata.Summary, error) { return l, nil }

func TestListShowsTheUploadedScenarioAndProject(t *testing.T) {
	env := newSessionsEnv(t)
	data := golden(t, "scenario-project")
	sid := headerID(t, data)
	env.putOK(t, "mac", sid, ndjson, data)
	rows := env.listRows(t)
	if len(rows) != 1 || rows[0]["scenario"] != "code-writer" || rows[0]["project"] != "pi-agent" {
		t.Errorf("rows = %v, want the session with scenario code-writer and project pi-agent", rows)
	}
}
