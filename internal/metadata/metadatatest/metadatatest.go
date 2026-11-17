// Package metadatatest is the contract test suite every metadata.Store must pass.
package metadatatest

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/goldens"
	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

// Stores are the store under test and the session store it links its metadata to
// (both on the same storage).
type Stores struct {
	Metadata metadata.Store
	Sessions session.Store
}

// Run runs the contract tests against stores made by newStores.
// newStores is called once per subtest and must return empty stores.
func Run(t *testing.T, newStores func(t *testing.T) Stores) {
	tests := []struct {
		name string
		fn   func(t *testing.T, s Stores)
	}{
		{"GetOfUnknownSessionReturnsErrNotFound", testGetOfUnknownSessionReturnsErrNotFound},
		{"ReplaceWithoutSessionRecordFailsAndStoresNothing", testReplaceWithoutSessionRecordFailsAndStoresNothing},
		{"ReplaceStoresEveryField", testReplaceStoresEveryField},
		{"MissingPartsStayMissing", testMissingPartsStayMissing},
		{"ClearedTagsStayCleared", testClearedTagsStayCleared},
		{"LocalProjectRoundTrips", testLocalProjectRoundTrips},
		{"SamplesAreNotStored", testSamplesAreNotStored},
		{"ReplaceLeavesNothingOfThePreviousMeta", testReplaceLeavesNothingOfThePreviousMeta},
		{"SessionsWithDifferentIDsAreSeparate", testSessionsWithDifferentIDsAreSeparate},
		{"IDPartsRoundTripUnchanged", testIDPartsRoundTripUnchanged},
		{"GoldenSessionsRoundTrip", testGoldenSessionsRoundTrip},
		{"ConcurrentReplacesLeaveOneWholeMeta", testConcurrentReplacesLeaveOneWholeMeta},
		{"ListOfEmptyStoreIsEmpty", testListOfEmptyStoreIsEmpty},
		{"ListSummarizesEverySession", testListSummarizesEverySession},
		{"ListIsNewestUploadFirstWithTiesOrderedByID", testListIsNewestUploadFirstWithTiesOrderedByID},
		{"ListShowsLatestUploadAndMetadata", testListShowsLatestUploadAndMetadata},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t, newStores(t))
		})
	}
}

var t0 = time.Date(2026, 11, 16, 9, 0, 0, 123_000_000, time.UTC)

func testGetOfUnknownSessionReturnsErrNotFound(t *testing.T, s Stores) {
	mustReplace(t, s, full(t, "mac", "other"))
	_, err := s.Metadata.Get(t.Context(), id(t, "mac", "s1"))
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("Get: got %v, want ErrNotFound", err)
	}
}

func testReplaceWithoutSessionRecordFailsAndStoresNothing(t *testing.T, s Stores) {
	mustReplace(t, s, full(t, "mac", "other"))
	m := full(t, "mac", "s1")
	if err := s.Metadata.Replace(t.Context(), m); !errors.Is(err, metadata.ErrNoSession) {
		t.Errorf("Replace without a session record: got %v, want ErrNoSession", err)
	}
	if _, err := s.Metadata.Get(t.Context(), m.ID); !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("Get after the failed Replace: got %v, want ErrNotFound", err)
	}
}

func testReplaceStoresEveryField(t *testing.T, s Stores) {
	m := full(t, "mac", "s1")
	mustReplace(t, s, m)
	assertMeta(t, mustGet(t, s, m.ID), m)
}

func testMissingPartsStayMissing(t *testing.T, s Stores) {
	m := minimal(t, "mac", "s1")
	mustReplace(t, s, m)
	got := mustGet(t, s, m.ID)
	assertMeta(t, got, m)
	if got.Metrics != nil {
		t.Errorf("Metrics = %+v, want nil", got.Metrics)
	}
}

func testClearedTagsStayCleared(t *testing.T, s Stores) {
	m := minimal(t, "mac", "s1")
	m.Metadata.Tags = &sessionparser.Tags{Tags: []string{}, Source: "command", At: t0}
	mustReplace(t, s, m)
	got := mustGet(t, s, m.ID)
	if got.Metadata.Tags == nil || len(got.Metadata.Tags.Tags) != 0 {
		t.Errorf("Tags = %+v, want a cleared tag list (not nil)", got.Metadata.Tags)
	}
	assertMeta(t, got, m)
}

func testLocalProjectRoundTrips(t *testing.T, s Stores) {
	m := minimal(t, "mac", "s1")
	m.Metadata.Project = &sessionparser.Project{Name: "p", At: t0, Source: sessionparser.ProjectSource{Kind: sessionparser.ProjectLocal, Dir: "/Users/me/p"}}
	mustReplace(t, s, m)
	assertMeta(t, mustGet(t, s, m.ID), m)
}

func testSamplesAreNotStored(t *testing.T, s Stores) {
	m := full(t, "mac", "s1")
	mustReplace(t, s, m)
	got := mustGet(t, s, m.ID)
	if got.Metrics == nil {
		t.Fatal("Metrics = nil, want the stored totals")
	}
	if smp := got.Metrics.Samples; len(smp.TTFTMs) != 0 || len(smp.LatencyMs) != 0 || len(smp.ToolMs) != 0 {
		t.Errorf("Samples = %+v, want empty", smp)
	}
}

func testReplaceLeavesNothingOfThePreviousMeta(t *testing.T, s Stores) {
	mustReplace(t, s, full(t, "mac", "s1"))
	second := minimal(t, "mac", "s1")
	second.ParserVersion = "v0.2.0"
	mustReplace(t, s, second)
	assertMeta(t, mustGet(t, s, second.ID), second)
}

func testSessionsWithDifferentIDsAreSeparate(t *testing.T, s Stores) {
	metas := []metadata.Meta{
		full(t, "mac", "s1"),
		minimal(t, "Mac", "s1"), // machineId differs in case
		full(t, "mac", "S1"),    // sessionId differs in case
		minimal(t, "linux", "s1"),
	}
	for i := range metas {
		metas[i].Metadata.CWD = fmt.Sprintf("/work/%d", i)
		mustReplace(t, s, metas[i])
	}
	for _, m := range metas {
		assertMeta(t, mustGet(t, s, m.ID), m)
	}
}

func testIDPartsRoundTripUnchanged(t *testing.T, s Stores) {
	m := minimal(t, "m.1_-", "a:b c%2F 中文 é")
	mustReplace(t, s, m)
	if got := mustGet(t, s, m.ID); got.ID != m.ID {
		t.Errorf("Get returned ID %+v, want %+v", got.ID, m.ID)
	}
}

func testGoldenSessionsRoundTrip(t *testing.T, s Stores) {
	for _, f := range goldens.All() {
		t.Run(f.Name, func(t *testing.T) {
			ps, err := sessionparser.Parse(bytes.NewReader(f.Data))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			md, err := ps.Metadata()
			if err != nil {
				t.Fatalf("Metadata: %v", err)
			}
			metrics, err := ps.Metrics()
			if err != nil {
				t.Fatalf("Metrics: %v", err)
			}
			m := metadata.Meta{ID: id(t, "mac", ps.Header.ID), Metadata: *md, Metrics: metrics, ParserVersion: "v0.1.0"}
			mustReplace(t, s, m)
			assertMeta(t, mustGet(t, s, m.ID), m)
		})
	}
}

func testConcurrentReplacesLeaveOneWholeMeta(t *testing.T, s Stores) {
	const n = 16
	metas := make([]metadata.Meta, n)
	for i := range n {
		if i%2 == 0 {
			metas[i] = full(t, "mac", "s1")
		} else {
			metas[i] = minimal(t, "mac", "s1")
		}
		metas[i].ParserVersion = fmt.Sprintf("v0.%d.0", i)
	}
	recordSession(t, s, metas[0].ID)
	var wg sync.WaitGroup
	for _, m := range metas {
		wg.Go(func() {
			if err := s.Metadata.Replace(t.Context(), m); err != nil {
				t.Errorf("Replace: %v", err)
			}
		})
	}
	wg.Wait()

	got := mustGet(t, s, metas[0].ID)
	for _, m := range metas {
		if m.ParserVersion == got.ParserVersion {
			assertMeta(t, got, m) // all of one Replace, nothing mixed in from another
			return
		}
	}
	t.Errorf("Get returned parser version %q, which no Replace wrote", got.ParserVersion)
}

func testListOfEmptyStoreIsEmpty(t *testing.T, s Stores) {
	if l := mustList(t, s); len(l) != 0 {
		t.Errorf("List = %+v, want empty", l)
	}
}

func testListSummarizesEverySession(t *testing.T, s Stores) {
	cleared := minimal(t, "mac", "cleared")
	cleared.Metadata.Tags = &sessionparser.Tags{Tags: []string{}, Source: "command", At: t0}
	metas := []metadata.Meta{full(t, "mac", "full"), minimal(t, "mac", "minimal"), cleared}
	var want []metadata.Summary
	for i, m := range metas {
		at := t0.Add(time.Duration(len(metas)-i) * time.Minute) // newest first in metas order
		mustUpload(t, s, m, at)
		want = append(want, summaryOf(m, at))
	}
	assertSummaries(t, mustList(t, s), want)
}

func testListIsNewestUploadFirstWithTiesOrderedByID(t *testing.T, s Stores) {
	t1, t2 := t0.Add(time.Minute), t0.Add(2*time.Minute)
	uploads := []struct {
		machineID, sessionID string
		at                   time.Time
	}{
		{"mac", "b", t1}, {"mac", "a", t1}, {"old", "x", t0}, {"linux", "z", t1},
		{"mac", "c", t2}, {"Mac", "a", t1}, {"mac", "B", t1},
	}
	for _, u := range uploads {
		mustUpload(t, s, minimal(t, u.machineID, u.sessionID), u.at)
	}
	// Byte order: upper case before lower case.
	want := []string{"mac/c", "Mac/a", "linux/z", "mac/B", "mac/a", "mac/b", "old/x"}
	var got []string
	for _, sm := range mustList(t, s) {
		got = append(got, sm.ID.MachineID+"/"+sm.ID.SessionID)
	}
	if !slices.Equal(got, want) {
		t.Errorf("List order = %v, want %v", got, want)
	}
}

func testListShowsLatestUploadAndMetadata(t *testing.T, s Stores) {
	t1, t2 := t0.Add(time.Minute), t0.Add(2*time.Minute)
	mustUpload(t, s, full(t, "mac", "s1"), t0)
	other := full(t, "mac", "s2")
	mustUpload(t, s, other, t1)
	reupload := minimal(t, "mac", "s1")
	mustUpload(t, s, reupload, t2)
	assertSummaries(t, mustList(t, s), []metadata.Summary{summaryOf(reupload, t2), summaryOf(other, t1)})
}

// summaryOf is the list row of a session uploaded at with metadata m.
func summaryOf(m metadata.Meta, at time.Time) metadata.Summary {
	sm := metadata.Summary{ID: m.ID, UploadedAt: at, Name: m.Metadata.Name, AgentVersion: m.Metadata.AgentVersion, Scenario: m.Metadata.Scenario}
	if p := m.Metadata.Project; p != nil {
		sm.Project = &p.Name
	}
	if m.Metadata.Tags != nil {
		sm.Tags = append([]string{}, m.Metadata.Tags.Tags...)
	}
	if m.Metrics != nil {
		sm.Metrics = &metadata.MetricsSummary{Tokens: m.Metrics.Tokens, CostUSD: m.Metrics.CostUSD, Turns: m.Metrics.Turns}
	}
	return sm
}

func mustList(t *testing.T, s Stores) []metadata.Summary {
	t.Helper()
	l, err := s.Metadata.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return l
}

// assertSummaries compares lists of rows: times as instants, and nil tags (no tags
// entry) apart from empty tags (cleared).
func assertSummaries(t *testing.T, got, want []metadata.Summary) {
	t.Helper()
	norm := func(l []metadata.Summary) []metadata.Summary {
		out := slices.Clone(l)
		for i := range out {
			out[i].UploadedAt = out[i].UploadedAt.UTC()
			if out[i].Tags != nil {
				out[i].Tags = append([]string{}, out[i].Tags...)
			}
		}
		return out
	}
	g, w := norm(got), norm(want)
	if !reflect.DeepEqual(g, w) {
		gj, _ := json.MarshalIndent(g, "", "  ")
		wj, _ := json.MarshalIndent(w, "", "  ")
		t.Errorf("List =\n%s\nwant\n%s", gj, wj)
	}
}

func id(t *testing.T, machineID, sessionID string) session.ID {
	t.Helper()
	sid, err := session.NewID(session.TypePi, machineID, sessionID)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return sid
}

func ptr[T any](v T) *T { return &v }

// full is a Meta with every field set.
func full(t *testing.T, machineID, sessionID string) metadata.Meta {
	t.Helper()
	return metadata.Meta{
		ID: id(t, machineID, sessionID),
		Metadata: sessionparser.Metadata{
			SessionID:     sessionID,
			CWD:           "/Users/me/project",
			StartedAt:     t0,
			FormatVersion: 3,
			ParentSession: ptr("/Users/me/.pi/sessions/parent.jsonl"),
			AgentVersion:  ptr("0.12.0"),
			Tags:          &sessionparser.Tags{Tags: []string{"b", "a", "中文"}, Source: "flag", At: t0.Add(time.Second)},
			Name:          ptr("design review"),
			Scenario:      ptr("code-writer"),
			Project: &sessionparser.Project{Name: "jarvis", At: t0.Add(1500 * time.Millisecond), Source: sessionparser.ProjectSource{
				Kind: sessionparser.ProjectRemote, Provider: "github", Repo: "me/jarvis", Path: "", Tag: "v1.0.0", Commit: "abc123",
			}},
			Models: []sessionparser.ModelChange{
				{Provider: "anthropic", ModelID: "claude-a", At: t0.Add(2 * time.Second)},
				{Provider: "openai", ModelID: "gpt-b", At: t0.Add(3 * time.Second)},
			},
			Errors: []sessionparser.AgentError{{Message: "extension failed", At: t0.Add(4 * time.Second)}},
		},
		Metrics: &sessionparser.Metrics{
			Tokens:  sessionparser.Tokens{Input: 1200, Output: 340, CacheRead: 5600, CacheWrite: 780},
			CostUSD: 0.123456,
			Turns:   7,
			Model:   sessionparser.ModelMetrics{Calls: 9, TTFTMsP50: ptr(812.5), LatencyMsP50: ptr(2400.25), LatencyMsMax: ptr(9100.0)},
			Tools: map[string]sessionparser.ToolMetrics{
				"bash": {Calls: 4, Errors: 1, MsP50: 120.5, MsMax: 3000},
				"read": {Calls: 2, MsP50: 3.25, MsMax: 4},
			},
			Compactions:        1,
			ContextPeakPercent: ptr(63.5),
			CacheMisses:        []sessionparser.CacheMiss{{Turn: 3, Expected: 5000, CacheRead: 100}},
			Samples: sessionparser.Samples{
				TTFTMs:    []float64{800, 825},
				LatencyMs: []float64{2400, 9100},
				ToolMs:    map[string][]float64{"bash": {120.5, 3000}},
			},
			Entries: 5,
			EntryID: "e42",
			At:      t0.Add(5 * time.Second),
		},
		ParserVersion: "v0.1.0",
	}
}

// minimal is a Meta with only the fields every session has.
func minimal(t *testing.T, machineID, sessionID string) metadata.Meta {
	t.Helper()
	return metadata.Meta{
		ID: id(t, machineID, sessionID),
		Metadata: sessionparser.Metadata{
			SessionID:     sessionID,
			CWD:           "/tmp",
			StartedAt:     t0,
			FormatVersion: 3,
		},
		ParserVersion: "v0.1.0",
	}
}

// recordSession records an upload of the session at t0, so metadata can link to it.
func recordSession(t *testing.T, s Stores, sid session.ID) {
	t.Helper()
	recordSessionAt(t, s, sid, t0)
}

func recordSessionAt(t *testing.T, s Stores, sid session.ID, at time.Time) {
	t.Helper()
	u := session.Upload{ID: sid, Blob: blob.Info{Hash: sha256.Sum256([]byte(sid.String())), Size: 1}, UploadedAt: at}
	if err := s.Sessions.RecordUpload(t.Context(), u); err != nil {
		t.Fatalf("RecordUpload(%s): %v", sid, err)
	}
}

// mustUpload does what an accepted upload at at does: records the session, then
// replaces its metadata with m.
func mustUpload(t *testing.T, s Stores, m metadata.Meta, at time.Time) {
	t.Helper()
	recordSessionAt(t, s, m.ID, at)
	if err := s.Metadata.Replace(t.Context(), m); err != nil {
		t.Fatalf("Replace(%s): %v", m.ID, err)
	}
}

// mustReplace records the session, then replaces its metadata with m.
func mustReplace(t *testing.T, s Stores, m metadata.Meta) {
	t.Helper()
	recordSession(t, s, m.ID)
	if err := s.Metadata.Replace(t.Context(), m); err != nil {
		t.Fatalf("Replace(%s): %v", m.ID, err)
	}
}

func mustGet(t *testing.T, s Stores, sid session.ID) metadata.Meta {
	t.Helper()
	m, err := s.Metadata.Get(t.Context(), sid)
	if err != nil {
		t.Fatalf("Get(%s): %v", sid, err)
	}
	return m
}

// assertMeta checks that got is want as stored: without samples, times compared as
// instants, and nil and empty slices and maps treated alike (except a cleared tag list,
// which stays a non-nil Tags).
func assertMeta(t *testing.T, got, want metadata.Meta) {
	t.Helper()
	g, w := normalize(got), normalize(want)
	if !reflect.DeepEqual(g, w) {
		gj, _ := json.MarshalIndent(g, "", "  ")
		wj, _ := json.MarshalIndent(w, "", "  ")
		t.Errorf("meta =\n%s\nwant\n%s", gj, wj)
	}
}

// normalize returns a copy of m in a form that compares with reflect.DeepEqual.
func normalize(m metadata.Meta) metadata.Meta {
	md := m.Metadata
	md.StartedAt = md.StartedAt.UTC()
	if md.Tags != nil {
		tags := *md.Tags
		tags.Tags = emptyToNil(slices.Clone(tags.Tags))
		tags.At = tags.At.UTC()
		md.Tags = &tags
	}
	if md.Project != nil {
		p := *md.Project
		p.At = p.At.UTC()
		md.Project = &p
	}
	md.Models = emptyToNil(slices.Clone(md.Models))
	for i := range md.Models {
		md.Models[i].At = md.Models[i].At.UTC()
	}
	md.Errors = emptyToNil(slices.Clone(md.Errors))
	for i := range md.Errors {
		md.Errors[i].At = md.Errors[i].At.UTC()
	}
	m.Metadata = md

	if m.Metrics != nil {
		mt := *m.Metrics
		mt.Samples = sessionparser.Samples{}
		mt.CacheMisses = emptyToNil(slices.Clone(mt.CacheMisses))
		mt.Tools = maps.Clone(mt.Tools)
		if len(mt.Tools) == 0 {
			mt.Tools = nil
		}
		mt.At = mt.At.UTC()
		m.Metrics = &mt
	}
	return m
}

func emptyToNil[S ~[]E, E any](s S) S {
	if len(s) == 0 {
		return nil
	}
	return s
}
