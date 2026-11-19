// Package metadata holds the parsed metadata of sessions and its storage.
package metadata

import (
	"context"
	"errors"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/session"
)

var (
	// ErrNotFound is returned when a session has no stored metadata.
	ErrNotFound = errors.New("session metadata not found")
	// ErrNoSession is returned when metadata is stored for a session that has no session record.
	ErrNoSession = errors.New("no session record")
)

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
	// Every metadata belongs to exactly one session record: Replace returns an error
	// wrapping ErrNoSession, and stores nothing, when the session was never recorded.
	Replace(ctx context.Context, m Meta) error

	// Get returns the metadata of a session, with Metrics.Samples empty.
	// It returns an error wrapping ErrNotFound when the session has none.
	Get(ctx context.Context, id session.ID) (Meta, error)

	// List returns a summary of every session with metadata, newest upload first.
	// Sessions with the same upload time are ordered by ID: type, machineId, then
	// sessionId, each compared byte for byte. It is empty, not an error, without sessions.
	List(ctx context.Context) ([]Summary, error)
}

// Summary is a session's row in the session list.
type Summary struct {
	ID session.ID
	// UploadedAt is the time of the session's last accepted upload (its session record's).
	UploadedAt time.Time
	// LastActivityAt is the metadata's LastActivityAt: the newest user or assistant message.
	LastActivityAt time.Time
	Name           *string
	AgentVersion   *string
	Scenario       *string
	// Project is the project's name; nil without a project.
	Project *string
	// Tags are nil without a tags entry, and empty (not nil) when cleared.
	Tags []string
	// Metrics are nil when the session has none.
	Metrics *MetricsSummary
}

// MetricsSummary are the metrics totals shown in the session list.
type MetricsSummary struct {
	Tokens  sessionparser.Tokens
	CostUSD float64
	Turns   int
}
