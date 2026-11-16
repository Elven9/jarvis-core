// Package blobtest is the contract test suite every blob.Store must pass.
package blobtest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/Elven9/jarvis-core/internal/blob"
)

// Run runs the contract tests against stores made by newStore.
// newStore is called once per subtest and must return an empty store.
func Run(t *testing.T, newStore func(t *testing.T) blob.Store) {
	tests := []struct {
		name string
		fn   func(t *testing.T, s blob.Store)
	}{
		{"PutReturnsSHA256AndSize", testPutReturnsSHA256AndSize},
		{"OpenReturnsTheBytesPut", testOpenReturnsTheBytesPut},
		{"PutStreamsLargeContent", testPutStreamsLargeContent},
		{"PutOfStoredContentReturnsSameInfo", testPutOfStoredContentReturnsSameInfo},
		{"OpenOfUnknownHashReturnsErrNotFound", testOpenOfUnknownHashReturnsErrNotFound},
		{"PutWithFailingReaderStoresNothing", testPutWithFailingReaderStoresNothing},
		{"PutWithCanceledContextFails", testPutWithCanceledContextFails},
		{"ConcurrentPutsOfSameContentAllSucceed", testConcurrentPutsOfSameContentAllSucceed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t, newStore(t))
		})
	}
}

func testPutReturnsSHA256AndSize(t *testing.T, s blob.Store) {
	content := []byte("{\"type\":\"session\"}\n")
	info, err := s.Put(t.Context(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	want := blob.Info{Hash: sha256.Sum256(content), Size: int64(len(content))}
	if info != want {
		t.Errorf("Put = %+v, want %+v", info, want)
	}
}

func testOpenReturnsTheBytesPut(t *testing.T, s blob.Store) {
	contents := map[string][]byte{
		"text":                []byte("line 1\nline 2\n"),
		"binary":              {0x00, 0xff, '\r', '\n', 0xe4, 0xb8},
		"no trailing newline": []byte("{}"),
	}
	for name, content := range contents {
		t.Run(name, func(t *testing.T) {
			info := mustPut(t, s, content)
			if got := mustRead(t, s, info.Hash); !bytes.Equal(got, content) {
				t.Errorf("Open returned %q, want %q", got, content)
			}
		})
	}
}

func testPutStreamsLargeContent(t *testing.T, s blob.Store) {
	content := make([]byte, 8<<20)
	_, _ = rand.NewChaCha8([32]byte{1}).Read(content)
	// Small reads make sure the store reads until EOF and does not rely on one Read.
	info, err := s.Put(t.Context(), &chunkReader{r: bytes.NewReader(content), n: 4096})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if info.Size != int64(len(content)) || info.Hash != sha256.Sum256(content) {
		t.Fatalf("Put = %+v, want size %d and the content's sha256", info, len(content))
	}
	if got := mustRead(t, s, info.Hash); !bytes.Equal(got, content) {
		t.Error("Open did not return the bytes put")
	}
}

func testPutOfStoredContentReturnsSameInfo(t *testing.T, s blob.Store) {
	content := []byte("same\n")
	first := mustPut(t, s, content)
	second := mustPut(t, s, content)
	if first != second {
		t.Errorf("second Put = %+v, want %+v", second, first)
	}
	if got := mustRead(t, s, second.Hash); !bytes.Equal(got, content) {
		t.Errorf("Open returned %q, want %q", got, content)
	}
}

func testOpenOfUnknownHashReturnsErrNotFound(t *testing.T, s blob.Store) {
	mustPut(t, s, []byte("other"))
	rc, err := s.Open(t.Context(), sha256.Sum256([]byte("never stored")))
	if err == nil {
		_ = rc.Close()
	}
	if !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Open: got %v, want ErrNotFound", err)
	}
}

func testPutWithFailingReaderStoresNothing(t *testing.T, s blob.Store) {
	errBoom := errors.New("connection reset")
	partial := []byte("partial upl")
	r := io.MultiReader(bytes.NewReader(partial), &failingReader{err: errBoom})
	if _, err := s.Put(t.Context(), r); !errors.Is(err, errBoom) {
		t.Fatalf("Put: got %v, want an error wrapping the reader's error", err)
	}
	rc, err := s.Open(t.Context(), sha256.Sum256(partial))
	if err == nil {
		_ = rc.Close()
	}
	if !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Open of the partial content: got %v, want ErrNotFound", err)
	}
}

func testPutWithCanceledContextFails(t *testing.T, s blob.Store) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	content := []byte("canceled")
	if _, err := s.Put(ctx, bytes.NewReader(content)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put: got %v, want context.Canceled", err)
	}
	rc, err := s.Open(t.Context(), sha256.Sum256(content))
	if err == nil {
		_ = rc.Close()
	}
	if !errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Open: got %v, want ErrNotFound", err)
	}
}

func testConcurrentPutsOfSameContentAllSucceed(t *testing.T, s blob.Store) {
	content := bytes.Repeat([]byte("concurrent\n"), 10_000)
	want := blob.Info{Hash: sha256.Sum256(content), Size: int64(len(content))}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			info, err := s.Put(t.Context(), bytes.NewReader(content))
			if err != nil || info != want {
				t.Errorf("Put = %+v, %v; want %+v", info, err, want)
			}
		})
	}
	wg.Wait()
	if got := mustRead(t, s, want.Hash); !bytes.Equal(got, content) {
		t.Error("Open did not return the bytes put")
	}
}

func mustPut(t *testing.T, s blob.Store, content []byte) blob.Info {
	t.Helper()
	info, err := s.Put(t.Context(), bytes.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	return info
}

func mustRead(t *testing.T, s blob.Store, h blob.Hash) []byte {
	t.Helper()
	rc, err := s.Open(t.Context(), h)
	if err != nil {
		t.Fatalf("Open(%s): %v", h, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read blob %s: %v", h, err)
	}
	return data
}

type chunkReader struct {
	r io.Reader
	n int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(p) > c.n {
		p = p[:c.n]
	}
	return c.r.Read(p)
}

type failingReader struct{ err error }

func (f *failingReader) Read([]byte) (int, error) { return 0, f.err }
