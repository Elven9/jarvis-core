// Package session holds the session identity and storage types.
package session

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Type is the kind of agent session (the client that produced it).
type Type string

// TypePi is the pi coding agent. It is the only type accepted in v0.1.
const TypePi Type = "pi"

// MaxIDPartLen is the maximum length in bytes of a machineId or sessionId.
const MaxIDPartLen = 256

// ErrInvalidID is wrapped by every error about an invalid session ID.
var ErrInvalidID = errors.New("invalid session id")

var machineIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ID is the global session identity: (type, machineId, sessionId).
// All parts are case-sensitive and compared byte for byte.
// Build one with NewID or ParseID so it is always valid.
type ID struct {
	Type      Type
	MachineID string
	SessionID string
}

// NewID validates the parts and returns the ID.
// The parts must already be percent-decoded.
func NewID(typ Type, machineID, sessionID string) (ID, error) {
	if err := validateType(typ); err != nil {
		return ID{}, err
	}
	if err := ValidateMachineID(machineID); err != nil {
		return ID{}, err
	}
	if err := ValidateSessionID(sessionID); err != nil {
		return ID{}, err
	}
	return ID{Type: typ, MachineID: machineID, SessionID: sessionID}, nil
}

// ParseID parses the string form "type:machineId:sessionId".
// Type and machineId cannot contain ':', so everything after the
// second colon is the sessionId, colons included.
func ParseID(s string) (ID, error) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) != 3 {
		return ID{}, fmt.Errorf("%w: %q is not in the form type:machineId:sessionId", ErrInvalidID, s)
	}
	return NewID(Type(parts[0]), parts[1], parts[2])
}

// String returns the string form "type:machineId:sessionId".
func (id ID) String() string {
	return string(id.Type) + ":" + id.MachineID + ":" + id.SessionID
}

func validateType(typ Type) error {
	if typ != TypePi {
		return fmt.Errorf("%w: unsupported type %q", ErrInvalidID, string(typ))
	}
	return nil
}

// ValidateMachineID checks a percent-decoded machineId.
func ValidateMachineID(machineID string) error {
	switch {
	case len(machineID) > MaxIDPartLen:
		return fmt.Errorf("%w: machineId is longer than %d bytes", ErrInvalidID, MaxIDPartLen)
	case !machineIDPattern.MatchString(machineID):
		return fmt.Errorf("%w: machineId must match %s", ErrInvalidID, machineIDPattern)
	case machineID == "." || machineID == "..":
		return fmt.Errorf("%w: machineId must not be %q", ErrInvalidID, machineID)
	}
	return nil
}

// ValidateSessionID checks a percent-decoded sessionId.
func ValidateSessionID(sessionID string) error {
	switch {
	case sessionID == "":
		return fmt.Errorf("%w: sessionId is empty", ErrInvalidID)
	case len(sessionID) > MaxIDPartLen:
		return fmt.Errorf("%w: sessionId is longer than %d bytes", ErrInvalidID, MaxIDPartLen)
	case sessionID == "." || sessionID == "..":
		return fmt.Errorf("%w: sessionId must not be %q", ErrInvalidID, sessionID)
	case !utf8.ValidString(sessionID):
		return fmt.Errorf("%w: sessionId is not valid UTF-8", ErrInvalidID)
	case strings.ContainsRune(sessionID, '/'):
		return fmt.Errorf("%w: sessionId must not contain '/'", ErrInvalidID)
	case strings.ContainsFunc(sessionID, unicode.IsControl):
		return fmt.Errorf("%w: sessionId must not contain control characters", ErrInvalidID)
	}
	return nil
}
