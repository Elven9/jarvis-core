// Package metadatatest is the contract test suite every metadata.Store must pass.
package metadatatest

import (
	"bytes"
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

	"github.com/Elven9/jarvis-core/internal/goldens"
	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

// Run runs the contract tests against stores made by newStore.
// newStore is called once per subtest and must return an empty store.
func Run(t *testing.T, newStore func(t *testing.T) metadata.Store) {
	tests := []struct {
		name string
		fn   func(t *testing.T, s metadata.Store)
	}{
		{"GetOfUnknownSessionReturnsErrNotFound", testGetOfUnknownSessionReturnsErrNotFound},
		{"ReplaceStoresEveryField", testReplaceStoresEveryField},
		{"MissingPartsStayMissing", testMissingPartsStayMissing},
		{"ClearedTagsStayCleared", testClearedTagsStayCleared},
		{"SamplesAreNotStored", testSamplesAreNotStored},
		{"ReplaceLeavesNothingOfThePreviousMeta", testReplaceLeavesNothingOfThePreviousMeta},
		{"SessionsWithDifferentIDsAreSeparate", testSessionsWithDifferentIDsAreSeparate},
		{"IDPartsRoundTripUnchanged", testIDPartsRoundTripUnchanged},
		{"GoldenSessionsRoundTrip", testGoldenSessionsRoundTrip},
		{"ConcurrentReplacesLeaveOneWholeMeta", testConcurrentReplacesLeaveOneWholeMeta},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t, newStore(t))
		})
	}
}

var t0 = time.Date(2026, 11, 16, 9, 0, 0, 123_000_000, time.UTC)

func testGetOfUnknownSessionReturnsErrNotFound(t *testing.T, s metadata.Store) {
	mustReplace(t, s, full(t, "mac", "other"))
	_, err := s.Get(t.Context(), id(t, "mac", "s1"))
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Errorf("Get: got %v, want ErrNotFound", err)
	}
}

func testReplaceStoresEveryField(t *testing.T, s metadata.Store) {
	m := full(t, "mac", "s1")
	mustReplace(t, s, m)
	assertMeta(t, mustGet(t, s, m.ID), m)
}

func testMissingPartsStayMissing(t *testing.T, s metadata.Store) {
	m := minimal(t, "mac", "s1")
	mustReplace(t, s, m)
	got := mustGet(t, s, m.ID)
	assertMeta(t, got, m)
	if got.Metrics != nil {
		t.Errorf("Metrics = %+v, want nil", got.Metrics)
	}
}

func testClearedTagsStayCleared(t *testing.T, s metadata.Store) {
	m := minimal(t, "mac", "s1")
	m.Metadata.Tags = &sessionparser.Tags{Tags: []string{}, Source: "command", At: t0}
	mustReplace(t, s, m)
	got := mustGet(t, s, m.ID)
	if got.Metadata.Tags == nil || len(got.Metadata.Tags.Tags) != 0 {
		t.Errorf("Tags = %+v, want a cleared tag list (not nil)", got.Metadata.Tags)
	}
	assertMeta(t, got, m)
}

func testSamplesAreNotStored(t *testing.T, s metadata.Store) {
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

func testReplaceLeavesNothingOfThePreviousMeta(t *testing.T, s metadata.Store) {
	mustReplace(t, s, full(t, "mac", "s1"))
	second := minimal(t, "mac", "s1")
	second.ParserVersion = "v0.2.0"
	mustReplace(t, s, second)
	assertMeta(t, mustGet(t, s, second.ID), second)
}

func testSessionsWithDifferentIDsAreSeparate(t *testing.T, s metadata.Store) {
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

func testIDPartsRoundTripUnchanged(t *testing.T, s metadata.Store) {
	m := minimal(t, "m.1_-", "a:b c%2F 中文 é")
	mustReplace(t, s, m)
	if got := mustGet(t, s, m.ID); got.ID != m.ID {
		t.Errorf("Get returned ID %+v, want %+v", got.ID, m.ID)
	}
}

func testGoldenSessionsRoundTrip(t *testing.T, s metadata.Store) {
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

func testConcurrentReplacesLeaveOneWholeMeta(t *testing.T, s metadata.Store) {
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
	var wg sync.WaitGroup
	for _, m := range metas {
		wg.Go(func() {
			if err := s.Replace(t.Context(), m); err != nil {
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

func mustReplace(t *testing.T, s metadata.Store, m metadata.Meta) {
	t.Helper()
	if err := s.Replace(t.Context(), m); err != nil {
		t.Fatalf("Replace(%s): %v", m.ID, err)
	}
}

func mustGet(t *testing.T, s metadata.Store, sid session.ID) metadata.Meta {
	t.Helper()
	m, err := s.Get(t.Context(), sid)
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
