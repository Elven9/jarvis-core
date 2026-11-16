package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/goldens"
	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

const validHeader = `{"type":"session","version":3,"id":"s1","timestamp":"2026-11-16T09:00:00.000Z","cwd":"/tmp"}` + "\n"

func customEntry(customType, data string) string {
	return `{"type":"custom","customType":"` + customType + `","id":"e1","parentId":null,"timestamp":"2026-11-16T09:00:01.000Z","data":` + data + "}\n"
}

func (e *sessionsEnv) meta(t *testing.T, machineID, sessionID string) (metadata.Meta, error) {
	t.Helper()
	id, err := session.NewID(session.TypePi, machineID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return e.metas.Get(t.Context(), id)
}

// errorLogs are the log records at level ERROR with the given message.
func errorLogs(t *testing.T, logs *logLines, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range logs.records(t) {
		if r["level"] == "ERROR" && r["msg"] == msg {
			out = append(out, r)
		}
	}
	return out
}

func TestUploadStoresParsedMetadataOfEveryGoldenFile(t *testing.T) {
	env := newSessionsEnv(t)
	for _, f := range goldens.All() {
		t.Run(f.Name, func(t *testing.T) {
			sid := headerID(t, f.Data)
			env.putOK(t, "mac", sid, ndjson, f.Data)

			id, _ := session.NewID(session.TypePi, "mac", sid)
			want, err := metadata.Parse(bytes.NewReader(f.Data), id)
			if err != nil {
				t.Fatal(err)
			}
			got, err := env.meta(t, "mac", sid)
			if err != nil {
				t.Fatalf("metadata after upload: %v", err)
			}
			gm, wm := got.Metadata, want.Metadata
			if got.ParserVersion != metadata.ParserVersion() || gm.SessionID != sid ||
				!reflect.DeepEqual(gm.Name, wm.Name) || !reflect.DeepEqual(gm.AgentVersion, wm.AgentVersion) ||
				!reflect.DeepEqual(tagList(gm), tagList(wm)) || len(gm.Models) != len(wm.Models) {
				t.Errorf("stored metadata = %+v (parser %q), want what the parser returns: %+v (parser %q)",
					gm, got.ParserVersion, wm, metadata.ParserVersion())
			}
			if (got.Metrics == nil) != (want.Metrics == nil) {
				t.Fatalf("stored metrics = %v, want %v", got.Metrics, want.Metrics)
			}
			if got.Metrics != nil && (got.Metrics.Tokens != want.Metrics.Tokens || got.Metrics.CostUSD != want.Metrics.CostUSD || got.Metrics.Turns != want.Metrics.Turns) {
				t.Errorf("stored metrics totals = %+v, want %+v", got.Metrics, want.Metrics)
			}
		})
	}
}

func TestUploadOfSessionWithoutMetricsStoresNullMetrics(t *testing.T) {
	env := newSessionsEnv(t)
	data := golden(t, "no-metrics")
	sid := headerID(t, data)
	env.putOK(t, "mac", sid, ndjson, data)
	got, err := env.meta(t, "mac", sid)
	if err != nil || got.Metrics != nil {
		t.Errorf("metadata = %+v, %v; want stored with nil metrics", got, err)
	}
}

func TestUploadRejectsInvalidSessionWith422AndChangesNoRecord(t *testing.T) {
	cases := map[string]string{
		"not JSON":             "hello\n",
		"no header":            customEntry("x", "{}"),
		"old session format":   strings.Replace(validHeader, `"version":3`, `"version":2`, 1),
		"invalid entry line":   validHeader + `{"type":"message","parentId":null}` + "\n",
		"invalid metadata":     validHeader + customEntry("pi-agent.tags", `{"version":99,"tags":["a"],"source":"flag"}`),
		"invalid metrics":      validHeader + customEntry("pi-agent.metrics", `{"version":99}`),
		"header ID differs":    strings.Replace(validHeader, `"id":"s1"`, `"id":"s2"`, 1),
		"header ID long":       strings.Replace(validHeader, `"id":"s1"`, `"id":"`+strings.Repeat("é", 300)+`"`, 1),
		"header ID other case": strings.Replace(validHeader, `"id":"s1"`, `"id":"S1"`, 1),
	}
	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			env := newSessionsEnv(t)
			resp, body := env.put(t, "mac", "s1", ndjson, []byte(file))
			assertShortTextError(t, resp, body, http.StatusUnprocessableEntity)
			if !strings.HasPrefix(body, "invalid session") {
				t.Errorf("body = %q, want the parse error", body)
			}
			env.assertNothingStored(t, "mac", "s1")
			if _, err := env.meta(t, "mac", "s1"); !errors.Is(err, metadata.ErrNotFound) {
				t.Errorf("metadata: got %v, want ErrNotFound", err)
			}
			if logs := errorLogs(t, env.logs, "upload rejected"); len(logs) != 1 || logs[0]["session_id"] != "s1" || logs[0]["error"] == nil {
				t.Errorf("error logs = %v, want one rejection with the session ID and error", logs)
			}
		})
	}
}

func TestRejectedReuploadKeepsPreviousRecordAndMetadata(t *testing.T) {
	env := newSessionsEnv(t)
	good := piSession(t, "s1")
	env.putOK(t, "mac", "s1", ndjson, good)
	before, _ := env.stored(t, "mac", "s1")
	metaBefore, err := env.meta(t, "mac", "s1")
	if err != nil {
		t.Fatal(err)
	}

	resp, body := env.put(t, "mac", "s1", ndjson, []byte("hello\n"))
	assertShortTextError(t, resp, body, http.StatusUnprocessableEntity)

	rec, content := env.stored(t, "mac", "s1")
	if rec != before || !bytes.Equal(content, good) {
		t.Errorf("record = %+v, want the previous one %+v", rec, before)
	}
	if h, _ := env.sessions.History(t.Context(), rec.ID); len(h) != 1 {
		t.Errorf("history has %d uploads, want only the accepted one", len(h))
	}
	if m, err := env.meta(t, "mac", "s1"); err != nil || m.Metadata.Name == nil || *m.Metadata.Name != *metaBefore.Metadata.Name {
		t.Errorf("metadata = %+v, %v; want the previous metadata", m, err)
	}
}

func TestUploadMetadataStoreFailureAnswers500AndKeepsTheNewRecord(t *testing.T) {
	env := newSessionsEnv(t)
	env = newSessionsEnvWith(t, env.blobs, env.sessions, failingMetadata{})
	sessionFile := piSession(t, "s1")

	resp, body := env.put(t, "mac", "s1", ndjson, sessionFile)
	assertShortTextError(t, resp, body, http.StatusInternalServerError)
	if strings.Contains(body, "metadata disk full") {
		t.Errorf("body %q leaks the internal error", body)
	}
	// No rollback across the write sequence: the record of step 3 stays.
	if _, content := env.stored(t, "mac", "s1"); !bytes.Equal(content, sessionFile) {
		t.Error("session record does not point at the upload")
	}
	if logs := errorLogs(t, env.logs, "upload failed"); len(logs) != 1 || !strings.Contains(logs[0]["error"].(string), "metadata disk full") {
		t.Errorf("error logs = %v, want one failure with the internal error", logs)
	}
}

func TestUploadStoreFailureWritesErrorLog(t *testing.T) {
	env := newSessionsEnv(t)
	env = newSessionsEnvWith(t, failingBlobs{}, env.sessions, env.metas)
	resp, body := env.put(t, "mac", "s1", ndjson, piSession(t, "s1"))
	assertShortTextError(t, resp, body, http.StatusInternalServerError)
	if logs := errorLogs(t, env.logs, "upload failed"); len(logs) != 1 {
		t.Errorf("error logs = %v, want one failure", logs)
	}
}

// tagList is the session's tags; nil without a tags entry, empty when cleared.
func tagList(md sessionparser.Metadata) []string {
	if md.Tags == nil {
		return nil
	}
	return append([]string{}, md.Tags.Tags...)
}

func TestReuploadReplacesMetadataWithNothingLeftOver(t *testing.T) {
	env := newSessionsEnv(t)
	first := golden(t, "tags") // tags and metrics
	sid := headerID(t, first)
	env.putOK(t, "mac", sid, ndjson, first)
	if m, err := env.meta(t, "mac", sid); err != nil || m.Metadata.Tags == nil || m.Metrics == nil {
		t.Fatalf("first upload: metadata = %+v, %v; want tags and metrics", m, err)
	}

	second := withHeaderID(t, golden(t, "no-metrics"), sid) // no tags, no metrics, no name
	env.putOK(t, "mac", sid, ndjson, second)

	got, err := env.meta(t, "mac", sid)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := session.NewID(session.TypePi, "mac", sid)
	want, err := metadata.Parse(bytes.NewReader(second), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Tags != nil || got.Metrics != nil || got.Metadata.Name != nil || got.Metadata.AgentVersion != nil {
		t.Errorf("after re-upload: tags %v, metrics %v, name %v, agent version %v; want all gone",
			got.Metadata.Tags, got.Metrics, got.Metadata.Name, got.Metadata.AgentVersion)
	}
	if got.Metadata.CWD != want.Metadata.CWD || !got.Metadata.StartedAt.Equal(want.Metadata.StartedAt) ||
		!reflect.DeepEqual(modelIDs(got.Metadata), modelIDs(want.Metadata)) {
		t.Errorf("after re-upload: metadata = %+v, want the second upload's %+v", got.Metadata, want.Metadata)
	}
}

func TestReuploadWithClearedTagsStoresClearedTags(t *testing.T) {
	env := newSessionsEnv(t)
	first := golden(t, "tags")
	sid := headerID(t, first)
	env.putOK(t, "mac", sid, ndjson, first)

	// The same session after /tags --clear: one more pi-agent.tags entry with an empty list.
	second := appendEntry(t, first, "pi-agent.tags", `{"version":1,"tags":[],"source":"command"}`)
	env.putOK(t, "mac", sid, ndjson, second)

	got, err := env.meta(t, "mac", sid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Metadata.Tags == nil || len(got.Metadata.Tags.Tags) != 0 || got.Metadata.Tags.Source != "command" {
		t.Errorf("tags = %+v, want the cleared list set by the command", got.Metadata.Tags)
	}
	if got.Metrics == nil {
		t.Error("metrics = nil, want the metrics the session still has")
	}
}

// appendEntry adds a custom entry after the file's last entry.
func appendEntry(t *testing.T, file []byte, customType, data string) []byte {
	t.Helper()
	lines := bytes.Split(bytes.TrimRight(file, "\n"), []byte("\n"))
	var last struct{ ID string }
	if err := json.Unmarshal(lines[len(lines)-1], &last); err != nil || last.ID == "" {
		t.Fatalf("last entry: %v", err)
	}
	entry := `{"type":"custom","customType":"` + customType + `","id":"appended1","parentId":"` + last.ID +
		`","timestamp":"2026-11-16T10:00:00.000Z","data":` + data + "}\n"
	return append(bytes.Clone(file), entry...)
}

func modelIDs(md sessionparser.Metadata) []string {
	var out []string
	for _, m := range md.Models {
		out = append(out, m.Provider+"/"+m.ModelID)
	}
	return out
}

type failingMetadata struct{}

func (failingMetadata) Replace(context.Context, metadata.Meta) error {
	return errors.New("metadata disk full")
}

func (failingMetadata) Get(context.Context, session.ID) (metadata.Meta, error) {
	return metadata.Meta{}, metadata.ErrNotFound
}

func (failingMetadata) List(context.Context) ([]metadata.Summary, error) { return nil, nil }

// headerID is the session ID in a session file's header.
func headerID(t *testing.T, file []byte) string {
	t.Helper()
	first, _, _ := bytes.Cut(file, []byte("\n"))
	var h struct{ ID string }
	if err := json.Unmarshal(first, &h); err != nil || h.ID == "" {
		t.Fatalf("header %q: %v", first, err)
	}
	return h.ID
}
