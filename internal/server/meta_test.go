package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

func (e *sessionsEnv) getMeta(t *testing.T, machineID, sessionID string) (*http.Response, []byte) {
	t.Helper()
	return e.getPath(t, "/v1/sessions/"+url.PathEscape(machineID)+"/"+url.PathEscape(sessionID)+"/meta")
}

// getMetaJSON gets the meta route and decodes its 200 JSON body.
func (e *sessionsEnv) getMetaJSON(t *testing.T, machineID, sessionID string) map[string]any {
	t.Helper()
	resp, body := e.getMeta(t, machineID, sessionID)
	return decodeJSON(t, resp, body)
}

func decodeJSON(t *testing.T, resp *http.Response, body []byte) map[string]any {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var v map[string]any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("body %q is not JSON: %v", body, err)
	}
	return v
}

func TestMetaAnswersEveryFieldInItsJSONShape(t *testing.T) {
	at := time.Date(2026, 11, 16, 9, 0, 0, 123_000_000, time.UTC)
	p := func(s string) *string { return &s }
	f := func(v float64) *float64 { return &v }
	m := metadata.Meta{
		Metadata: sessionparser.Metadata{
			SessionID: "s1", CWD: "/w", StartedAt: at, FormatVersion: 3,
			ParentSession: p("/p.jsonl"), AgentVersion: p("0.12.0"), Name: p("review"), Scenario: p("demo"),
			Project: &sessionparser.Project{Name: "jarvis", At: at, Source: sessionparser.ProjectSource{Kind: sessionparser.ProjectLocal, Dir: "/w"}},
			Tags:    &sessionparser.Tags{Tags: []string{"b", "a"}, Source: "flag", At: at},
			Models:  []sessionparser.ModelChange{{Provider: "anthropic", ModelID: "claude", At: at}},
			Errors:  []sessionparser.AgentError{{Message: "boom", At: at}},
		},
		Metrics: &sessionparser.Metrics{
			Tokens:  sessionparser.Tokens{Input: 1, Output: 2, CacheRead: 3, CacheWrite: 4},
			CostUSD: 0.5, Turns: 6,
			Model:              sessionparser.ModelMetrics{Calls: 7, TTFTMsP50: f(8.5), LatencyMsP50: f(9), LatencyMsMax: nil},
			Tools:              map[string]sessionparser.ToolMetrics{"bash": {Calls: 2, Errors: 1, MsP50: 3.5, MsMax: 4}},
			Compactions:        1,
			ContextPeakPercent: f(42.5),
			CacheMisses:        []sessionparser.CacheMiss{{Turn: 2, Expected: 100, CacheRead: 10}},
			Samples:            sessionparser.Samples{TTFTMs: []float64{1}},
			Entries:            3, EntryID: "e9", At: at,
		},
		ParserVersion: "v0.1.0",
	}
	env := newSessionsEnvWith(t, failingBlobs{}, &oneSession{}, fixedMeta{m})
	got := env.getMetaJSON(t, "mac", "s1")

	var want map[string]any
	if err := json.Unmarshal([]byte(`{
		"metadata": {
			"sessionId": "s1", "cwd": "/w", "startedAt": "2026-11-16T09:00:00.123Z", "formatVersion": 3,
			"parentSession": "/p.jsonl", "agentVersion": "0.12.0", "name": "review", "scenario": "demo",
			"project": {"name": "jarvis", "at": "2026-11-16T09:00:00.123Z", "source": {
				"kind": "local", "dir": "/w", "provider": null, "repo": null, "path": null, "tag": null, "commit": null}},
			"tags": {"tags": ["b", "a"], "source": "flag", "at": "2026-11-16T09:00:00.123Z"},
			"models": [{"provider": "anthropic", "modelId": "claude", "at": "2026-11-16T09:00:00.123Z"}],
			"errors": [{"message": "boom", "at": "2026-11-16T09:00:00.123Z"}]
		},
		"metrics": {
			"tokens": {"input": 1, "output": 2, "cacheRead": 3, "cacheWrite": 4},
			"costUsd": 0.5, "turns": 6,
			"model": {"calls": 7, "ttftMsP50": 8.5, "latencyMsP50": 9, "latencyMsMax": null},
			"tools": {"bash": {"calls": 2, "errors": 1, "msP50": 3.5, "msMax": 4}},
			"compactions": 1, "contextPeakPercent": 42.5,
			"cacheMisses": [{"turn": 2, "expected": 100, "cacheRead": 10}],
			"entries": 3, "entryId": "e9", "at": "2026-11-16T09:00:00.123Z"
		},
		"parserVersion": "v0.1.0"
	}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		gj, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("body =\n%s\nwant the documented shape", gj)
	}
}

func TestMetaAnswersAbsentPartsAsNullAndEmptyLists(t *testing.T) {
	m := metadata.Meta{Metadata: sessionparser.Metadata{SessionID: "s1", CWD: "/w", StartedAt: time.UnixMilli(0), FormatVersion: 3}, ParserVersion: "v0.1.0"}
	env := newSessionsEnvWith(t, failingBlobs{}, &oneSession{}, fixedMeta{m})
	got := env.getMetaJSON(t, "mac", "s1")
	md := got["metadata"].(map[string]any)
	for _, k := range []string{"parentSession", "agentVersion", "tags", "name", "scenario", "project"} {
		if v, ok := md[k]; !ok || v != nil {
			t.Errorf("metadata.%s = %v (present %v), want null", k, v, ok)
		}
	}
	for _, k := range []string{"models", "errors"} {
		if v, ok := md[k].([]any); !ok || len(v) != 0 {
			t.Errorf("metadata.%s = %v, want []", k, md[k])
		}
	}
	if v, ok := got["metrics"]; !ok || v != nil {
		t.Errorf("metrics = %v (present %v), want null", v, ok)
	}
}

func TestMetaAnswersClearedTagsAsEmptyList(t *testing.T) {
	m := metadata.Meta{Metadata: sessionparser.Metadata{SessionID: "s1", Tags: &sessionparser.Tags{Source: "command"}}}
	env := newSessionsEnvWith(t, failingBlobs{}, &oneSession{}, fixedMeta{m})
	got := env.getMetaJSON(t, "mac", "s1")
	tags, _ := got["metadata"].(map[string]any)["tags"].(map[string]any)
	if list, ok := tags["tags"].([]any); !ok || len(list) != 0 {
		t.Errorf("metadata.tags = %v, want an object with tags []", tags)
	}
}

func TestMetaReturnsWhatTheUploadStored(t *testing.T) {
	env := newSessionsEnv(t)
	data := golden(t, "tags")
	sid := headerID(t, data)
	env.putOK(t, "mac", sid, ndjson, data)

	got := env.getMetaJSON(t, "mac", sid)
	stored, err := env.meta(t, "mac", sid)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, _ := json.Marshal(toMetaJSON(stored))
	var want map[string]any
	_ = json.Unmarshal(wantJSON, &want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("meta route differs from the stored metadata")
	}
	md := got["metadata"].(map[string]any)
	if md["sessionId"] != sid || got["parserVersion"] != metadata.ParserVersion() || got["metrics"] == nil {
		t.Errorf("meta = %v, want session %s with metrics and parser %s", got, sid, metadata.ParserVersion())
	}
	if tags := md["tags"].(map[string]any)["tags"]; !reflect.DeepEqual(tags, []any{"helloworld"}) {
		t.Errorf("tags = %v, want [helloworld]", tags)
	}
}

func TestMetaOfUploadWithoutMetricsHasNullMetrics(t *testing.T) {
	env := newSessionsEnv(t)
	data := golden(t, "no-metrics")
	sid := headerID(t, data)
	env.putOK(t, "mac", sid, ndjson, data)
	if got := env.getMetaJSON(t, "mac", sid); got["metrics"] != nil {
		t.Errorf("metrics = %v, want null", got["metrics"])
	}
}

func TestMetaOfUnknownSessionIsNotFoundProblem(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "other", ndjson, piSession(t, "other"))
	resp, body := env.getMeta(t, "mac", "s1")
	p := assertProblem(t, resp, body, http.StatusNotFound)
	if detail, _ := p["detail"].(string); !strings.Contains(detail, "pi:mac:s1") {
		t.Errorf("detail = %q, want it to name the session", detail)
	}
}

// A session recorded before uploads were parsed (v0.1) has a record but no metadata.
func TestMetaOfSessionWithoutMetadataIsNotFoundProblem(t *testing.T) {
	env := newSessionsEnv(t)
	id, _ := session.NewID(session.TypePi, "mac", "s1")
	if err := env.sessions.RecordUpload(t.Context(), session.Upload{ID: id, UploadedAt: env.now}); err != nil {
		t.Fatal(err)
	}
	resp, body := env.getMeta(t, "mac", "s1")
	assertProblem(t, resp, body, http.StatusNotFound)
}

func TestMetaRejectsInvalidIDsWithProblem(t *testing.T) {
	env := newSessionsEnv(t)
	for _, path := range []string{"/v1/sessions/a%20b/s1/meta", "/v1/sessions/mac/a%00b/meta", "/v1/sessions/mac/s1/meta?type=claude"} {
		t.Run(path, func(t *testing.T) {
			resp, body := env.getPath(t, path)
			assertProblem(t, resp, body, http.StatusBadRequest)
		})
	}
}

func TestMetaStoreFailureIsInternalProblemWithoutDetails(t *testing.T) {
	env := newSessionsEnvWith(t, failingBlobs{}, &oneSession{}, brokenMeta{})
	resp, body := env.getMeta(t, "mac", "s1")
	p := assertProblem(t, resp, body, http.StatusInternalServerError)
	if _, ok := p["detail"]; ok {
		t.Errorf("problem = %v, want no detail for an internal error", p)
	}
}

// fixedMeta is a metadata.Store that has m for every session.
type fixedMeta struct{ m metadata.Meta }

func (fixedMeta) Replace(context.Context, metadata.Meta) error { return nil }

func (f fixedMeta) Get(_ context.Context, id session.ID) (metadata.Meta, error) {
	m := f.m
	m.ID = id
	return m, nil
}

func (fixedMeta) List(context.Context) ([]metadata.Summary, error) { return nil, nil }

type brokenMeta struct{}

func (brokenMeta) List(context.Context) ([]metadata.Summary, error) {
	return nil, errors.New("database corrupt")
}

func (brokenMeta) Replace(context.Context, metadata.Meta) error { return errors.New("broken") }

func (brokenMeta) Get(context.Context, session.ID) (metadata.Meta, error) {
	return metadata.Meta{}, errors.New("database corrupt")
}

func TestMetaAnswersTheUploadedScenarioAndProject(t *testing.T) {
	env := newSessionsEnv(t)
	file := validHeader +
		`{"type":"custom","customType":"pi-agent.scenario","id":"e1","parentId":null,"timestamp":"2026-11-16T09:00:01.000Z","data":{"version":1,"name":"code-writer"}}` + "\n" +
		`{"type":"custom","customType":"pi-agent.project","id":"e2","parentId":"e1","timestamp":"2026-11-16T09:00:02.000Z","data":{"version":1,"name":"jarvis","source":{"kind":"remote","provider":"github","repo":"me/jarvis","path":"","tag":"v1","commit":"abc"}}}` + "\n"
	env.putOK(t, "mac", "s1", ndjson, []byte(file))

	md := env.getMetaJSON(t, "mac", "s1")["metadata"].(map[string]any)
	project, _ := md["project"].(map[string]any)
	source, _ := project["source"].(map[string]any)
	if md["scenario"] != "code-writer" || project["name"] != "jarvis" || source["kind"] != "remote" || source["repo"] != "me/jarvis" || source["commit"] != "abc" {
		t.Errorf("scenario = %v, project = %v; want code-writer and the remote jarvis project", md["scenario"], project)
	}
}
