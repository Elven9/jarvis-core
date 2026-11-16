// Package sessiontest is the contract test suite every session.Store must pass.
package sessiontest

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/session"
)

// Run runs the contract tests against stores made by newStore.
// newStore is called once per subtest and must return an empty store.
func Run(t *testing.T, newStore func(t *testing.T) session.Store) {
	tests := []struct {
		name string
		fn   func(t *testing.T, s session.Store)
	}{
		{"GetOfUnknownSessionReturnsErrNotFound", testGetOfUnknownSessionReturnsErrNotFound},
		{"FirstUploadCreatesRecord", testFirstUploadCreatesRecord},
		{"ReuploadOverwritesAndKeepsFirstUploadTime", testReuploadOverwritesAndKeepsFirstUploadTime},
		{"HistoryKeepsEveryUploadInOrder", testHistoryKeepsEveryUploadInOrder},
		{"HistoryOfUnknownSessionIsEmpty", testHistoryOfUnknownSessionIsEmpty},
		{"SessionsWithDifferentIDsAreSeparate", testSessionsWithDifferentIDsAreSeparate},
		{"IDPartsRoundTripUnchanged", testIDPartsRoundTripUnchanged},
		{"ConcurrentUploadsAreAllRecordedAndLastWins", testConcurrentUploadsAreAllRecordedAndLastWins},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t, newStore(t))
		})
	}
}

var t0 = time.Date(2026, 11, 16, 9, 0, 0, 123_000_000, time.UTC)

func testGetOfUnknownSessionReturnsErrNotFound(t *testing.T, s session.Store) {
	mustRecord(t, s, upload(t, "mac", "other", "x", t0))
	_, err := s.Get(t.Context(), id(t, "mac", "s1"))
	if !errors.Is(err, session.ErrNotFound) {
		t.Errorf("Get: got %v, want ErrNotFound", err)
	}
}

func testFirstUploadCreatesRecord(t *testing.T, s session.Store) {
	u := upload(t, "mac", "s1", "v1", t0)
	mustRecord(t, s, u)
	want := session.Record{ID: u.ID, Blob: u.Blob, FirstUploadedAt: t0, LastUploadedAt: t0}
	assertRecord(t, mustGet(t, s, u.ID), want)
}

func testReuploadOverwritesAndKeepsFirstUploadTime(t *testing.T, s session.Store) {
	first := upload(t, "mac", "s1", "v1", t0)
	second := upload(t, "mac", "s1", "v2", t0.Add(time.Minute))
	mustRecord(t, s, first)
	mustRecord(t, s, second)
	want := session.Record{ID: first.ID, Blob: second.Blob, FirstUploadedAt: t0, LastUploadedAt: second.UploadedAt}
	assertRecord(t, mustGet(t, s, first.ID), want)
}

func testHistoryKeepsEveryUploadInOrder(t *testing.T, s session.Store) {
	uploads := []session.Upload{
		upload(t, "mac", "s1", "v1", t0),
		upload(t, "mac", "s1", "v1", t0.Add(time.Second)), // same content again
		upload(t, "mac", "s1", "v2", t0.Add(2*time.Second)),
	}
	for _, u := range uploads {
		mustRecord(t, s, u)
	}
	assertHistory(t, mustHistory(t, s, uploads[0].ID), uploads)
}

func testHistoryOfUnknownSessionIsEmpty(t *testing.T, s session.Store) {
	mustRecord(t, s, upload(t, "mac", "other", "x", t0))
	if h := mustHistory(t, s, id(t, "mac", "s1")); len(h) != 0 {
		t.Errorf("History = %+v, want empty", h)
	}
}

func testSessionsWithDifferentIDsAreSeparate(t *testing.T, s session.Store) {
	uploads := []session.Upload{
		upload(t, "mac", "s1", "a", t0),
		upload(t, "Mac", "s1", "b", t0.Add(time.Second)),   // machineId differs in case
		upload(t, "mac", "S1", "c", t0.Add(2*time.Second)), // sessionId differs in case
		upload(t, "linux", "s1", "d", t0.Add(3*time.Second)),
	}
	for _, u := range uploads {
		mustRecord(t, s, u)
	}
	for _, u := range uploads {
		want := session.Record{ID: u.ID, Blob: u.Blob, FirstUploadedAt: u.UploadedAt, LastUploadedAt: u.UploadedAt}
		assertRecord(t, mustGet(t, s, u.ID), want)
		assertHistory(t, mustHistory(t, s, u.ID), []session.Upload{u})
	}
}

func testIDPartsRoundTripUnchanged(t *testing.T, s session.Store) {
	u := upload(t, "m.1_-", "a:b c%2F 中文 é", "v1", t0)
	mustRecord(t, s, u)
	if got := mustGet(t, s, u.ID); got.ID != u.ID {
		t.Errorf("Get returned ID %+v, want %+v", got.ID, u.ID)
	}
}

func testConcurrentUploadsAreAllRecordedAndLastWins(t *testing.T, s session.Store) {
	const n = 16
	sid := id(t, "mac", "s1")
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			u := upload(t, "mac", "s1", fmt.Sprint(i), t0.Add(time.Duration(i)*time.Second))
			if err := s.RecordUpload(t.Context(), u); err != nil {
				t.Errorf("RecordUpload: %v", err)
			}
		})
	}
	wg.Wait()

	history := mustHistory(t, s, sid)
	if len(history) != n {
		t.Fatalf("History has %d uploads, want %d", len(history), n)
	}
	first, last := history[0], history[n-1]
	want := session.Record{ID: sid, Blob: last.Blob, FirstUploadedAt: first.UploadedAt, LastUploadedAt: last.UploadedAt}
	assertRecord(t, mustGet(t, s, sid), want)
}

func id(t *testing.T, machineID, sessionID string) session.ID {
	t.Helper()
	sid, err := session.NewID(session.TypePi, machineID, sessionID)
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	return sid
}

func upload(t *testing.T, machineID, sessionID, content string, at time.Time) session.Upload {
	t.Helper()
	return session.Upload{
		ID:         id(t, machineID, sessionID),
		Blob:       blob.Info{Hash: sha256.Sum256([]byte(content)), Size: int64(len(content))},
		UploadedAt: at,
	}
}

func mustRecord(t *testing.T, s session.Store, u session.Upload) {
	t.Helper()
	if err := s.RecordUpload(t.Context(), u); err != nil {
		t.Fatalf("RecordUpload: %v", err)
	}
}

func mustGet(t *testing.T, s session.Store, sid session.ID) session.Record {
	t.Helper()
	r, err := s.Get(t.Context(), sid)
	if err != nil {
		t.Fatalf("Get(%s): %v", sid, err)
	}
	return r
}

func mustHistory(t *testing.T, s session.Store, sid session.ID) []session.Upload {
	t.Helper()
	h, err := s.History(t.Context(), sid)
	if err != nil {
		t.Fatalf("History(%s): %v", sid, err)
	}
	return h
}

func assertRecord(t *testing.T, got, want session.Record) {
	t.Helper()
	if got.ID != want.ID || got.Blob != want.Blob ||
		!got.FirstUploadedAt.Equal(want.FirstUploadedAt) || !got.LastUploadedAt.Equal(want.LastUploadedAt) {
		t.Errorf("record = %+v, want %+v", got, want)
	}
}

func assertHistory(t *testing.T, got, want []session.Upload) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("History = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Blob != want[i].Blob || !got[i].UploadedAt.Equal(want[i].UploadedAt) {
			t.Errorf("History[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
