package metadatatest

import (
	"context"
	"fmt"
	"sync"
	"testing"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

// TestMemoryStorePassesContract checks the suite itself against a minimal in-memory store.
func TestMemoryStorePassesContract(t *testing.T) {
	Run(t, func(*testing.T) Stores {
		sessions := &memSessions{ids: map[session.ID]bool{}}
		return Stores{Metadata: &memStore{sessions: sessions, metas: map[session.ID]metadata.Meta{}}, Sessions: sessions}
	})
}

type memStore struct {
	sessions *memSessions
	mu       sync.Mutex
	metas    map[session.ID]metadata.Meta
}

func (m *memStore) Replace(ctx context.Context, meta metadata.Meta) error {
	if _, err := m.sessions.Get(ctx, meta.ID); err != nil {
		return fmt.Errorf("replace %s: %w", meta.ID, metadata.ErrNoSession)
	}
	if meta.Metrics != nil {
		mt := *meta.Metrics
		mt.Samples = sessionparser.Samples{}
		meta.Metrics = &mt
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metas[meta.ID] = meta
	return nil
}

func (m *memStore) Get(_ context.Context, id session.ID) (metadata.Meta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, ok := m.metas[id]
	if !ok {
		return metadata.Meta{}, fmt.Errorf("get %s: %w", id, metadata.ErrNotFound)
	}
	return meta, nil
}

// memSessions only knows which sessions were recorded.
type memSessions struct {
	mu  sync.Mutex
	ids map[session.ID]bool
}

func (m *memSessions) RecordUpload(_ context.Context, u session.Upload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ids[u.ID] = true
	return nil
}

func (m *memSessions) Get(_ context.Context, id session.ID) (session.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.ids[id] {
		return session.Record{}, session.ErrNotFound
	}
	return session.Record{ID: id}, nil
}

func (m *memSessions) History(context.Context, session.ID) ([]session.Upload, error) {
	return nil, nil
}
