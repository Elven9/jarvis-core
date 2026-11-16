package session

import (
	"context"
	"errors"
	"time"

	"github.com/Elven9/jarvis-core/internal/blob"
)

// ErrNotFound is returned when no session has the requested ID.
var ErrNotFound = errors.New("session not found")

// Upload is one accepted upload of a session: which blob it stored and when.
type Upload struct {
	ID         ID
	Blob       blob.Info
	UploadedAt time.Time
}

// Record is the current state of a session: its latest upload and when it was first seen.
type Record struct {
	ID              ID
	Blob            blob.Info
	FirstUploadedAt time.Time
	LastUploadedAt  time.Time
}

// Store keeps the session records and the upload history.
// Implementations must be safe for concurrent use and keep times to at least millisecond precision.
type Store interface {
	// RecordUpload makes u the current state of its session (last write wins) and
	// appends it to the session's upload history, even when the blob is unchanged.
	// Both happen together or not at all.
	RecordUpload(ctx context.Context, u Upload) error

	// Get returns the current record of a session.
	// It returns an error wrapping ErrNotFound when the session was never uploaded.
	Get(ctx context.Context, id ID) (Record, error)

	// History returns every upload of a session, oldest first.
	// It is empty, not an error, when the session was never uploaded.
	History(ctx context.Context, id ID) ([]Upload, error)
}
