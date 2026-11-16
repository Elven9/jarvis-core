package blobtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/Elven9/jarvis-core/internal/blob"
)

// TestMemoryStorePassesContract checks the suite itself against a minimal in-memory store.
func TestMemoryStorePassesContract(t *testing.T) {
	Run(t, func(*testing.T) blob.Store { return &memStore{blobs: map[blob.Hash][]byte{}} })
}

type memStore struct {
	mu    sync.Mutex
	blobs map[blob.Hash][]byte
}

func (m *memStore) Put(ctx context.Context, r io.Reader) (blob.Info, error) {
	if err := ctx.Err(); err != nil {
		return blob.Info{}, err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return blob.Info{}, fmt.Errorf("read blob: %w", err)
	}
	info := blob.Info{Hash: sha256.Sum256(data), Size: int64(len(data))}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blobs[info.Hash] = data
	return info, nil
}

func (m *memStore) Open(_ context.Context, h blob.Hash) (io.ReadCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.blobs[h]
	if !ok {
		return nil, fmt.Errorf("open %s: %w", h, blob.ErrNotFound)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
