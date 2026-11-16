// Package metadata holds the parsed metadata of sessions and its storage.
package metadata

import (
	"context"
	"errors"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/session"
)

// ErrNotFound is returned when a session has no stored metadata.
var ErrNotFound = errors.New("session metadata not found")

// Meta is what the session parser returned for a session's latest accepted upload.
type Meta struct {
	ID       session.ID
	Metadata sessionparser.Metadata
	// Metrics are the session's metrics totals; nil when it has no pi-agent.metrics entry.
	// Samples are not stored.
	Metrics *sessionparser.Metrics
	// ParserVersion is the version of the session-parser module that produced Metadata and Metrics.
	ParserVersion string
}

// Store keeps the parsed metadata of each session.
// Implementations must be safe for concurrent use and keep times to at least millisecond precision.
type Store interface {
	// Replace makes m the metadata of its session. Nothing of what was stored for the
	// session before remains. It happens wholly or not at all.
	Replace(ctx context.Context, m Meta) error

	// Get returns the metadata of a session, with Metrics.Samples empty.
	// It returns an error wrapping ErrNotFound when the session has none.
	Get(ctx context.Context, id session.ID) (Meta, error)
}
