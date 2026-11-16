package fsblob

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/blob/blobtest"
)

func newStore(t *testing.T) (*Store, string, string) {
	t.Helper()
	root := t.TempDir()
	blobDir, tmpDir := filepath.Join(root, "blobs"), filepath.Join(root, "tmp")
	s, err := New(blobDir, tmpDir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, blobDir, tmpDir
}

func TestStorePassesContract(t *testing.T) {
	blobtest.Run(t, func(t *testing.T) blob.Store {
		s, _, _ := newStore(t)
		return s
	})
}

func TestNewCreatesDirectories(t *testing.T) {
	_, blobDir, tmpDir := newStore(t)
	for _, dir := range []string{blobDir, tmpDir} {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Errorf("%s: %v, want a directory", dir, err)
		}
	}
}

func TestPutStoresBlobAtShardedHashPath(t *testing.T) {
	s, blobDir, _ := newStore(t)
	content := []byte("{\"type\":\"session\"}\n")
	if _, err := s.Put(t.Context(), bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:])
	got, err := os.ReadFile(filepath.Join(blobDir, name[:2], name))
	if err != nil {
		t.Fatalf("blob file: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("blob file = %q, want the original bytes %q", got, content)
	}
}

func TestStoredBlobsAreReadOnly(t *testing.T) {
	s, blobDir, _ := newStore(t)
	info, err := s.Put(t.Context(), bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatal(err)
	}
	name := info.Hash.String()
	fi, err := os.Stat(filepath.Join(blobDir, name[:2], name))
	if err != nil {
		t.Fatal(err)
	}
	if mode := fi.Mode().Perm(); mode != 0o444 {
		t.Errorf("blob mode = %v, want -r--r--r--", mode)
	}
}

func TestPutOfStoredContentKeepsOneFile(t *testing.T) {
	s, blobDir, tmpDir := newStore(t)
	for range 3 {
		if _, err := s.Put(t.Context(), bytes.NewReader([]byte("same"))); err != nil {
			t.Fatal(err)
		}
	}
	if files := listFiles(t, blobDir); len(files) != 1 {
		t.Errorf("blob files = %v, want 1", files)
	}
	if files := listFiles(t, tmpDir); len(files) != 0 {
		t.Errorf("temp files left: %v", files)
	}
}

func TestFailedPutLeavesNoFiles(t *testing.T) {
	s, blobDir, tmpDir := newStore(t)
	r := io.MultiReader(bytes.NewReader([]byte("partial")), errReader{})
	if _, err := s.Put(t.Context(), r); err == nil {
		t.Fatal("Put succeeded, want the reader's error")
	}
	if files := append(listFiles(t, blobDir), listFiles(t, tmpDir)...); len(files) != 0 {
		t.Errorf("files left after a failed Put: %v", files)
	}
}

func TestNewRemovesLeftoverTempFiles(t *testing.T) {
	_, blobDir, tmpDir := newStore(t)
	leftover := filepath.Join(tmpDir, "blob-123")
	other := filepath.Join(tmpDir, "not-ours")
	for _, p := range []string{leftover, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New(blobDir, tmpDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(leftover); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("leftover temp file still there: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("a file the store did not create was removed: %v", err)
	}
}

func TestBlobsSurviveReopening(t *testing.T) {
	s, blobDir, tmpDir := newStore(t)
	info, err := s.Put(t.Context(), bytes.NewReader([]byte("kept")))
	if err != nil {
		t.Fatal(err)
	}
	s2, err := New(blobDir, tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := s2.Open(t.Context(), info.Hash)
	if err != nil {
		t.Fatalf("Open after reopening: %v", err)
	}
	defer func() { _ = rc.Close() }()
	if got, _ := io.ReadAll(rc); string(got) != "kept" {
		t.Errorf("Open = %q, want \"kept\"", got)
	}
}

func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("connection reset") }
