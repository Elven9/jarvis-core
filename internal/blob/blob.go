// Package blob defines the content-addressed store for raw session bytes.
package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

// ErrNotFound is returned when no blob has the requested hash.
var ErrNotFound = errors.New("blob not found")

// Hash is the sha256 of a blob's content. It is the blob's address.
type Hash [sha256.Size]byte

// String returns the hash as lowercase hex.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// Info describes a stored blob.
type Info struct {
	Hash Hash
	Size int64
}

// Store keeps blobs addressed by the sha256 of their content.
// Implementations must be safe for concurrent use.
type Store interface {
	// Put streams r into the store and returns its hash and size.
	// Putting content that is already stored is not an error and returns the same Info.
	// If r or ctx fails, Put returns an error wrapping it and stores nothing.
	Put(ctx context.Context, r io.Reader) (Info, error)

	// Open returns the content of the blob with hash h.
	// It returns an error wrapping ErrNotFound when there is no such blob.
	Open(ctx context.Context, h Hash) (io.ReadCloser, error)
}
