package server

import (
	"bytes"
	"encoding/json"
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

func (e *sessionsEnv) getParsed(t *testing.T, machineID, sessionID string) (*http.Response, []byte) {
	t.Helper()
	return e.getPath(t, "/v1/sessions/"+url.PathEscape(machineID)+"/"+url.PathEscape(sessionID))
}

// jsonOf is v marshaled and decoded again, to compare with a decoded body.
func jsonOf(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestReadParsedAnswersEveryGroupTheParserReadsFromTheUpload(t *testing.T) {
	for _, name := range []string{"tags", "agent-version", "named", "no-metrics"} {
		t.Run(name, func(t *testing.T) {
			env := newSessionsEnv(t)
			data := golden(t, name)
			sid := headerID(t, data)
			env.putOK(t, "mac", sid, ndjson, data)

			resp, body := env.getParsed(t, "mac", sid)
			got := decodeJSON(t, resp, body)
			id, err := session.NewID(session.TypePi, "mac", sid)
			if err != nil {
				t.Fatal(err)
			}
			p, err := metadata.ParseAll(bytes.NewReader(data), id)
			if err != nil {
				t.Fatal(err)
			}
			if want := jsonOf(t, toParsedJSON(p)); !reflect.DeepEqual(got, want) {
				t.Errorf("body differs from what the parser reads from the upload")
			}
			if got["parserVersion"] != metadata.ParserVersion() {
				t.Errorf("parserVersion = %v, want %s", got["parserVersion"], metadata.ParserVersion())
			}
		})
	}
}

func TestReadParsedAnswersCodemodeScriptsOfToolCalls(t *testing.T) {
	env := newSessionsEnv(t)
	data := golden(t, "tags")
	sid := headerID(t, data)
	env.putOK(t, "mac", sid, ndjson, data)
	resp, body := env.getParsed(t, "mac", sid)
	got := decodeJSON(t, resp, body)

	calls, _ := got["toolCalls"].([]any)
	var scripts int
	for _, c := range calls {
		if cm, _ := c.(map[string]any)["codemode"].(map[string]any); cm != nil && cm["code"] != "" {
			scripts++
		}
	}
	if len(calls) == 0 || scripts == 0 {
		t.Errorf("toolCalls has %d calls, %d with a codemode script; want both > 0", len(calls), scripts)
	}
}

func TestReadParsedAnswersTheLatestUpload(t *testing.T) {
	env := newSessionsEnv(t)
	full := piSession(t, "s1")
	env.putOK(t, "mac", "s1", ndjson, full)
	env.putOK(t, "mac", "s1", ndjson, firstLines(full, 1))
	resp, body := env.getParsed(t, "mac", "s1")
	if calls := decodeJSON(t, resp, body)["toolCalls"]; !reflect.DeepEqual(calls, []any{}) {
		t.Errorf("toolCalls = %v, want [] from the header-only re-upload", calls)
	}
}

func TestReadParsedAnswersEveryFieldInItsJSONShape(t *testing.T) {
	at := time.Date(2026, 11, 16, 9, 0, 0, 123_000_000, time.UTC)
	p := func(s string) *string { return &s }
	f := func(v float64) *float64 { return &v }
	ref := 7
	parsed := metadata.Parsed{
		Metadata: sessionparser.Metadata{
			SessionID: "s1", CWD: "/w", StartedAt: at, FormatVersion: 3, Scenario: p("demo"),
			Project: &sessionparser.Project{Name: "jarvis", At: at, Source: sessionparser.ProjectSource{
				Kind: sessionparser.ProjectRemote, Provider: "github", Repo: "o/r", Path: "", Tag: "v1", Commit: "abc",
			}},
		},
		Metrics: &sessionparser.Metrics{
			Turns: 1, At: at,
			Samples: sessionparser.Samples{TTFTMs: []float64{1}, ToolMs: map[string][]float64{"read": {2}}},
		},
		ToolCalls: []sessionparser.ToolCall{
			{
				ID: "c1", Name: "codemode", Args: json.RawMessage(`{"code":"x"}`), EntryID: "e1", CalledAt: at, Ref: &ref,
				Result: &sessionparser.ToolResult{
					EntryID: "e2", Content: []sessionparser.ContentBlock{{Type: "text", Text: p("ok")}},
					Details: json.RawMessage(`{"k":1}`), At: at, DurationMs: f(5),
				},
				Codemode: &sessionparser.CodemodeScript{Code: "x", Intent: p("look"), Nested: []sessionparser.NestedCall{
					{ID: "c1/1", Name: "read", ArgsPreview: `{"path":"a"}`, Args: json.RawMessage(`{"path":"a"}`), Status: sessionparser.NestedOK, DurationMs: f(1)},
				}},
			},
			{ID: "c2", Name: "bash", Args: json.RawMessage(`{}`), ArgsError: p("bad"), EntryID: "e3", CalledAt: at},
		},
		Subagents: []sessionparser.Subagent{{
			ID: "a1", Scenario: "debugger", Kind: "task", Task: "t", StartedBy: "model", StartedAt: at, TaskMessageID: "m1", EntryID: "e4",
			Result: &sessionparser.SubagentResult{ResultID: "r1", Status: sessionparser.SubagentOK, Summary: "done", EndedAt: at, Delivered: true, EntryID: "e5", MessageID: "m2"},
		}},
		Skills: sessionparser.Skills{
			Available: []sessionparser.Skill{{Name: "golang", Description: "Go", Location: "/s/SKILL.md"}},
			Used: []sessionparser.SkillUse{{Name: "golang", Location: "/s/SKILL.md", Loads: []sessionparser.SkillLoad{
				{Via: sessionparser.SkillViaCodemode, CallID: "c1/1", EntryID: "e1", At: at},
			}}},
		},
		ProjectInstructions: sessionparser.ProjectInstructions{
			Docs: []sessionparser.Instruction{{Path: "docs/a.md", When: "always", Reads: []sessionparser.InstructionRead{
				{Tool: sessionparser.ToolProjectRead, CallID: "c3", EntryID: "e6", At: at, Failed: true},
			}}},
		},
		ParserVersion: "v0.2.0",
	}
	got := jsonOf(t, toParsedJSON(parsed))

	var want map[string]any
	if err := json.Unmarshal([]byte(`{
		"metadata": {
			"sessionId": "s1", "cwd": "/w", "startedAt": "2026-11-16T09:00:00.123Z", "formatVersion": 3,
			"parentSession": null, "agentVersion": null, "tags": null, "name": null, "models": [], "errors": [],
			"scenario": "demo",
			"project": {"name": "jarvis", "at": "2026-11-16T09:00:00.123Z", "source": {
				"kind": "remote", "dir": null, "provider": "github", "repo": "o/r", "path": "", "tag": "v1", "commit": "abc"}}
		},
		"metrics": {
			"tokens": {"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0},
			"costUsd": 0, "turns": 1,
			"model": {"calls": 0, "ttftMsP50": null, "latencyMsP50": null, "latencyMsMax": null},
			"tools": {}, "compactions": 0, "contextPeakPercent": null, "cacheMisses": [],
			"entries": 0, "entryId": "", "at": "2026-11-16T09:00:00.123Z",
			"samples": {"ttftMs": [1], "latencyMs": [], "toolMs": {"read": [2]}}
		},
		"toolCalls": [
			{"id": "c1", "name": "codemode", "args": {"code": "x"}, "argsError": null, "entryId": "e1",
			 "calledAt": "2026-11-16T09:00:00.123Z", "ref": 7,
			 "result": {"entryId": "e2", "isError": false, "content": [{"type": "text", "text": "ok"}],
			            "details": {"k": 1}, "at": "2026-11-16T09:00:00.123Z", "durationMs": 5},
			 "codemode": {"code": "x", "intent": "look", "fullOutputPath": null, "nested": [
				{"id": "c1/1", "name": "read", "argsPreview": "{\"path\":\"a\"}", "args": {"path": "a"},
				 "status": "ok", "durationMs": 1, "error": null, "costUsd": null}]}},
			{"id": "c2", "name": "bash", "args": {}, "argsError": "bad", "entryId": "e3",
			 "calledAt": "2026-11-16T09:00:00.123Z", "ref": null, "result": null, "codemode": null}
		],
		"subagents": [
			{"id": "a1", "scenario": "debugger", "kind": "task", "task": "t", "project": null, "restartOf": null,
			 "startedBy": "model", "startedAt": "2026-11-16T09:00:00.123Z", "taskMessageId": "m1", "entryId": "e4",
			 "result": {"resultId": "r1", "status": "ok", "summary": "done", "reason": null, "detailKind": null,
			            "detail": null, "endedAt": "2026-11-16T09:00:00.123Z", "childSessionFile": null,
			            "delivered": true, "entryId": "e5", "messageId": "m2"}}
		],
		"skills": {
			"available": [{"name": "golang", "description": "Go", "location": "/s/SKILL.md"}],
			"used": [{"name": "golang", "location": "/s/SKILL.md", "loads": [
				{"via": "codemode", "callId": "c1/1", "entryId": "e1", "at": "2026-11-16T09:00:00.123Z", "failed": false}]}]
		},
		"projectInstructions": {
			"workflows": [],
			"docs": [{"path": "docs/a.md", "when": "always", "reads": [
				{"tool": "project_read", "inCodemode": false, "callId": "c3", "entryId": "e6",
				 "at": "2026-11-16T09:00:00.123Z", "failed": true}]}]
		},
		"parserVersion": "v0.2.0"
	}`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		gj, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("body =\n%s\nwant the documented shape", gj)
	}
}

func TestReadParsedAnswersALocalProjectWithItsDirOnly(t *testing.T) {
	parsed := metadata.Parsed{Metadata: sessionparser.Metadata{Project: &sessionparser.Project{
		Name: "p", Source: sessionparser.ProjectSource{Kind: sessionparser.ProjectLocal, Dir: "/p"},
	}}}
	src := jsonOf(t, toParsedJSON(parsed))["metadata"].(map[string]any)["project"].(map[string]any)["source"]
	want := map[string]any{"kind": "local", "dir": "/p", "provider": nil, "repo": nil, "path": nil, "tag": nil, "commit": nil}
	if !reflect.DeepEqual(src, want) {
		t.Errorf("project.source = %v, want %v", src, want)
	}
}

func TestReadParsedAnswersAbsentPartsAsNullAndEmptyLists(t *testing.T) {
	got := jsonOf(t, toParsedJSON(metadata.Parsed{}))
	md := got["metadata"].(map[string]any)
	for _, k := range []string{"scenario", "project"} {
		if v, ok := md[k]; !ok || v != nil {
			t.Errorf("metadata.%s = %v (present %v), want null", k, v, ok)
		}
	}
	if v, ok := got["metrics"]; !ok || v != nil {
		t.Errorf("metrics = %v (present %v), want null", v, ok)
	}
	empty := map[string]any{
		"toolCalls": got["toolCalls"], "subagents": got["subagents"],
		"skills.available": got["skills"].(map[string]any)["available"], "skills.used": got["skills"].(map[string]any)["used"],
		"projectInstructions.workflows": got["projectInstructions"].(map[string]any)["workflows"],
		"projectInstructions.docs":      got["projectInstructions"].(map[string]any)["docs"],
	}
	for k, v := range empty {
		if !reflect.DeepEqual(v, []any{}) {
			t.Errorf("%s = %v, want []", k, v)
		}
	}
}

func TestReadParsedOfStoredSessionTheParserRejectsIs422Problem(t *testing.T) {
	env := newSessionsEnv(t)
	// Uploads reject such a file, so store it directly, as a file an older parser accepted.
	info, err := env.blobs.Put(t.Context(), strings.NewReader(validHeader+orphanToolResult))
	if err != nil {
		t.Fatal(err)
	}
	id, err := session.NewID(session.TypePi, "mac", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.sessions.RecordUpload(t.Context(), session.Upload{ID: id, Blob: info, UploadedAt: env.now}); err != nil {
		t.Fatal(err)
	}

	resp, body := env.getParsed(t, "mac", "s1")
	p := assertProblem(t, resp, body, http.StatusUnprocessableEntity)
	if detail, _ := p["detail"].(string); !strings.Contains(detail, "pi:mac:s1") || !strings.Contains(detail, metadata.ParserVersion()) {
		t.Errorf("detail = %q, want it to name the session and the parser version", detail)
	}
}

func TestReadParsedOfUnknownSessionIsNotFoundProblem(t *testing.T) {
	env := newSessionsEnv(t)
	env.putOK(t, "mac", "other", ndjson, piSession(t, "other"))
	resp, body := env.getParsed(t, "mac", "s1")
	p := assertProblem(t, resp, body, http.StatusNotFound)
	if detail, _ := p["detail"].(string); !strings.Contains(detail, "pi:mac:s1") {
		t.Errorf("detail = %q, want it to name the session", detail)
	}
}

func TestReadParsedRejectsInvalidIDsWithProblem(t *testing.T) {
	env := newSessionsEnv(t)
	for _, path := range []string{"/v1/sessions/a%20b/s1", "/v1/sessions/mac/a%00b", "/v1/sessions/mac/a%zzb", "/v1/sessions/mac/s1?type=claude"} {
		t.Run(path, func(t *testing.T) {
			resp, body := env.getPath(t, path)
			if p := assertProblem(t, resp, body, http.StatusBadRequest); p["detail"] == nil {
				t.Errorf("problem = %v, want a detail saying what is wrong", p)
			}
		})
	}
}

func TestReadParsedStoreFailureIsInternalProblemWithoutDetails(t *testing.T) {
	env := newSessionsEnvWith(t, failingBlobs{}, &oneSession{}, nil)
	resp, body := env.getParsed(t, "mac", "s1")
	p := assertProblem(t, resp, body, http.StatusInternalServerError)
	if _, ok := p["detail"]; ok {
		t.Errorf("problem = %v, want no detail for an internal error", p)
	}
}
