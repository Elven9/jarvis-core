package sessiontest

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Elven9/jarvis-core/internal/session"
)

// TestMemoryStorePassesContract checks the suite itself against a minimal in-memory store.
func TestMemoryStorePassesContract(t *testing.T) {
	Run(t, func(*testing.T) session.Store { return &memStore{records: map[session.ID]session.Record{}} })
}

type memStore struct {
	mu      sync.Mutex
	records map[session.ID]session.Record
	history []session.Upload
}

func (m *memStore) RecordUpload(_ context.Context, u session.Upload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[u.ID]
	if !ok {
		r = session.Record{ID: u.ID, FirstUploadedAt: u.UploadedAt}
	}
	r.Blob, r.LastUploadedAt = u.Blob, u.UploadedAt
	m.records[u.ID] = r
	m.history = append(m.history, u)
	return nil
}

func (m *memStore) Get(_ context.Context, id session.ID) (session.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return session.Record{}, fmt.Errorf("get %s: %w", id, session.ErrNotFound)
	}
	return r, nil
}

func (m *memStore) History(_ context.Context, id session.ID) ([]session.Upload, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []session.Upload
	for _, u := range m.history {
		if u.ID == id {
			out = append(out, u)
		}
	}
	return out, nil
}
