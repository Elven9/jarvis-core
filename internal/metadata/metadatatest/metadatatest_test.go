package metadatatest

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

// TestMemoryStorePassesContract checks the suite itself against a minimal in-memory store.
func TestMemoryStorePassesContract(t *testing.T) {
	Run(t, func(*testing.T) Stores {
		sessions := &memSessions{last: map[session.ID]time.Time{}}
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

func (m *memStore) List(ctx context.Context) ([]metadata.Summary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []metadata.Summary{}
	for id, meta := range m.metas {
		rec, err := m.sessions.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		sm := metadata.Summary{ID: id, UploadedAt: rec.LastUploadedAt, Name: meta.Metadata.Name, AgentVersion: meta.Metadata.AgentVersion}
		if meta.Metadata.Tags != nil {
			sm.Tags = append([]string{}, meta.Metadata.Tags.Tags...)
		}
		if mt := meta.Metrics; mt != nil {
			sm.Metrics = &metadata.MetricsSummary{Tokens: mt.Tokens, CostUSD: mt.CostUSD, Turns: mt.Turns}
		}
		out = append(out, sm)
	}
	slices.SortFunc(out, func(a, b metadata.Summary) int {
		return cmp.Or(b.UploadedAt.Compare(a.UploadedAt),
			cmp.Compare(a.ID.Type, b.ID.Type), cmp.Compare(a.ID.MachineID, b.ID.MachineID), cmp.Compare(a.ID.SessionID, b.ID.SessionID))
	})
	return out, nil
}

// memSessions only knows which sessions were recorded, and when last.
type memSessions struct {
	mu   sync.Mutex
	last map[session.ID]time.Time
}

func (m *memSessions) RecordUpload(_ context.Context, u session.Upload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.last[u.ID] = u.UploadedAt
	return nil
}

func (m *memSessions) Get(_ context.Context, id session.ID) (session.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	at, ok := m.last[id]
	if !ok {
		return session.Record{}, session.ErrNotFound
	}
	return session.Record{ID: id, LastUploadedAt: at}, nil
}

func (m *memSessions) History(context.Context, session.ID) ([]session.Upload, error) {
	return nil, nil
}
