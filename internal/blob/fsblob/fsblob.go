// Package fsblob is a blob.Store on the local filesystem.
//
// Blobs are kept uncompressed at <blobDir>/<first 2 hex chars>/<sha256 hex>.
// Put writes to a temp file in tmpDir, then renames it into place, so tmpDir
// must be on the same filesystem as blobDir. Stored files are read-only.
package fsblob

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Elven9/jarvis-core/internal/blob"
)

const (
	tempPrefix = "blob-"
	dirMode    = 0o755
	blobMode   = 0o444
)

// Store keeps blobs in a directory tree. It is safe for concurrent use.
type Store struct {
	blobDir string
	tmpDir  string
}

var _ blob.Store = (*Store)(nil)

// New creates blobDir and tmpDir if needed and removes temp files a crashed
// Put left in tmpDir. Only one Store may use a tmpDir at a time.
func New(blobDir, tmpDir string) (*Store, error) {
	for _, dir := range []string{blobDir, tmpDir} {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return nil, fmt.Errorf("create blob store directory: %w", err)
		}
	}
	if err := removeTempFiles(tmpDir); err != nil {
		return nil, err
	}
	return &Store{blobDir: blobDir, tmpDir: tmpDir}, nil
}

func removeTempFiles(tmpDir string) error {
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("list temp files: %w", err)
	}
	for _, e := range entries {
		if e.Type().IsRegular() && strings.HasPrefix(e.Name(), tempPrefix) {
			if err := os.Remove(filepath.Join(tmpDir, e.Name())); err != nil {
				return fmt.Errorf("remove leftover temp file: %w", err)
			}
		}
	}
	return nil
}

// Put streams r to a temp file while hashing it, then renames the file into place.
// If the blob already exists the temp file is dropped.
func (s *Store) Put(ctx context.Context, r io.Reader) (blob.Info, error) {
	if err := ctx.Err(); err != nil {
		return blob.Info{}, fmt.Errorf("put blob: %w", err)
	}
	tmp, err := os.CreateTemp(s.tmpDir, tempPrefix+"*")
	if err != nil {
		return blob.Info{}, fmt.Errorf("put blob: %w", err)
	}
	stored := false
	defer func() {
		if !stored {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), &ctxReader{ctx: ctx, r: r})
	if err != nil {
		return blob.Info{}, fmt.Errorf("put blob: %w", err)
	}
	if err := tmp.Chmod(blobMode); err != nil {
		return blob.Info{}, fmt.Errorf("put blob: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return blob.Info{}, fmt.Errorf("put blob: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return blob.Info{}, fmt.Errorf("put blob: %w", err)
	}

	info := blob.Info{Hash: blob.Hash(h.Sum(nil)), Size: size}
	path := s.path(info.Hash)
	if _, err := os.Stat(path); err == nil {
		return info, nil // already stored; the deferred cleanup drops the temp file
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return blob.Info{}, fmt.Errorf("put blob %s: %w", info.Hash, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return blob.Info{}, fmt.Errorf("put blob %s: %w", info.Hash, err)
	}
	stored = true
	if err := syncDir(dir); err != nil {
		return blob.Info{}, fmt.Errorf("put blob %s: %w", info.Hash, err)
	}
	return info, nil
}

// Open returns the blob's file.
func (s *Store) Open(_ context.Context, h blob.Hash) (io.ReadCloser, error) {
	f, err := os.Open(s.path(h))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("open blob %s: %w", h, blob.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("open blob %s: %w", h, err)
	}
	return f, nil
}

func (s *Store) path(h blob.Hash) string {
	hex := h.String()
	return filepath.Join(s.blobDir, hex[:2], hex)
}

// syncDir makes a rename into dir durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

// ctxReader stops reading once ctx is done, so a canceled upload stops early.
type ctxReader struct {
	ctx context.Context //nolint:containedctx // scoped to one Put call
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
