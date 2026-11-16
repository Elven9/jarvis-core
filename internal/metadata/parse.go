package metadata

import (
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/session"
)

// ErrInvalidSession is wrapped by every error about an upload the parser does not accept:
// the file does not parse, its metadata or metrics are invalid, or its header ID is not
// the session's ID.
var ErrInvalidSession = errors.New("invalid session")

// parserModule is the module of the session parser, whose version is stored with the metadata.
const parserModule = "github.com/Elven9/pi-agent/go"

// ParserVersion is the version of the session-parser module built into this binary,
// or "unknown" when the binary has no module information.
var ParserVersion = sync.OnceValue(func() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path != parserModule {
				continue
			}
			if d.Replace != nil {
				d = d.Replace
			}
			return d.Version
		}
	}
	return "unknown"
})

// Parse reads the session file r of the session id and returns its metadata.
// It fails with an error wrapping ErrInvalidSession when the file does not parse, its
// metadata or metrics are invalid, or its header ID differs from id.SessionID; any other
// error is a read error.
func Parse(r io.Reader, id session.ID) (Meta, error) {
	s, err := sessionparser.Parse(r)
	if err != nil {
		if isReadError(err) {
			return Meta{}, fmt.Errorf("read session: %w", err)
		}
		return Meta{}, fmt.Errorf("%w: %w", ErrInvalidSession, err)
	}
	if s.Header.ID != id.SessionID {
		return Meta{}, fmt.Errorf("%w: header id %q differs from sessionId %q", ErrInvalidSession, s.Header.ID, id.SessionID)
	}
	md, err := s.Metadata()
	if err != nil {
		return Meta{}, fmt.Errorf("%w: metadata: %w", ErrInvalidSession, err)
	}
	metrics, err := s.Metrics()
	if err != nil {
		return Meta{}, fmt.Errorf("%w: metrics: %w", ErrInvalidSession, err)
	}
	return Meta{ID: id, Metadata: *md, Metrics: metrics, ParserVersion: ParserVersion()}, nil
}

// isReadError tells a failure to read the file from a file the parser rejects.
func isReadError(err error) bool {
	if errors.Is(err, sessionparser.ErrNoHeader) || errors.Is(err, sessionparser.ErrUnsupportedFormat) {
		return false
	}
	if _, ok := errors.AsType[*sessionparser.LineError](err); ok {
		return false
	}
	return true
}
