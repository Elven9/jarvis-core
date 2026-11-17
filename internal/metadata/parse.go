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
	s, err := parseSession(r, id)
	if err != nil {
		return Meta{}, err
	}
	md, err := s.Metadata()
	if err != nil {
		return Meta{}, invalid("metadata", err)
	}
	metrics, err := s.Metrics()
	if err != nil {
		return Meta{}, invalid("metrics", err)
	}
	return Meta{ID: id, Metadata: *md, Metrics: metrics, ParserVersion: ParserVersion()}, nil
}

// Parsed is every group the session parser reads from a session file.
type Parsed struct {
	ID       session.ID
	Metadata sessionparser.Metadata
	// Metrics are the totals and samples; nil when the session has no pi-agent.metrics entry.
	Metrics             *sessionparser.Metrics
	ToolCalls           []sessionparser.ToolCall
	Subagents           []sessionparser.Subagent
	Skills              sessionparser.Skills
	ProjectInstructions sessionparser.ProjectInstructions
	// ParserVersion is the version of the session-parser module that produced the groups.
	ParserVersion string
}

// ParseAll reads the session file r of the session id with every parser group. It fails
// like Parse, and with an error wrapping ErrInvalidSession when any group is invalid.
func ParseAll(r io.Reader, id session.ID) (Parsed, error) {
	s, err := parseSession(r, id)
	if err != nil {
		return Parsed{}, err
	}
	p := Parsed{ID: id, ParserVersion: ParserVersion()}
	md, err := s.Metadata()
	if err != nil {
		return Parsed{}, invalid("metadata", err)
	}
	p.Metadata = *md
	if p.Metrics, err = s.Metrics(); err != nil {
		return Parsed{}, invalid("metrics", err)
	}
	if p.ToolCalls, err = s.ToolCalls(); err != nil {
		return Parsed{}, invalid("tool calls", err)
	}
	if p.Subagents, err = s.Subagents(); err != nil {
		return Parsed{}, invalid("subagents", err)
	}
	skills, err := s.Skills()
	if err != nil {
		return Parsed{}, invalid("skills", err)
	}
	if skills != nil {
		p.Skills = *skills
	}
	pi, err := s.ProjectInstructions()
	if err != nil {
		return Parsed{}, invalid("project instructions", err)
	}
	if pi != nil {
		p.ProjectInstructions = *pi
	}
	return p, nil
}

// parseSession parses r and checks its header ID against id.
func parseSession(r io.Reader, id session.ID) (*sessionparser.Session, error) {
	s, err := sessionparser.Parse(r)
	if err != nil {
		if isReadError(err) {
			return nil, fmt.Errorf("read session: %w", err)
		}
		return nil, fmt.Errorf("%w: %w", ErrInvalidSession, err)
	}
	if s.Header.ID != id.SessionID {
		return nil, fmt.Errorf("%w: header id %q differs from sessionId %q", ErrInvalidSession, s.Header.ID, id.SessionID)
	}
	return s, nil
}

func invalid(group string, err error) error {
	return fmt.Errorf("%w: %s: %w", ErrInvalidSession, group, err)
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
