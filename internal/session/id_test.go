package session

import (
	"errors"
	"strings"
	"testing"
)

func TestNewIDAcceptsValidParts(t *testing.T) {
	id, err := NewID(TypePi, "mac-mini.home_1", "0193f1a2-7b3c-7d4e-8f90-1234567890ab")
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	want := ID{Type: TypePi, MachineID: "mac-mini.home_1", SessionID: "0193f1a2-7b3c-7d4e-8f90-1234567890ab"}
	if id != want {
		t.Errorf("got %+v, want %+v", id, want)
	}
}

func TestNewIDRejectsTypesOtherThanPi(t *testing.T) {
	for _, typ := range []Type{"", "claude", "PI", "Pi", "pi "} {
		t.Run(string(typ), func(t *testing.T) {
			_, err := NewID(typ, "m", "s")
			if !errors.Is(err, ErrInvalidID) {
				t.Errorf("NewID(%q): got %v, want ErrInvalidID", typ, err)
			}
		})
	}
}

func TestNewIDRejectsInvalidMachineID(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"slash":             "a/b",
		"colon":             "a:b",
		"space":             "a b",
		"percent":           "a%20b",
		"non-ascii":         "café",
		"dot":               ".",
		"dot dot":           "..",
		"longer than 256 B": strings.Repeat("a", MaxIDPartLen+1),
	}
	for name, machineID := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewID(TypePi, machineID, "s")
			if !errors.Is(err, ErrInvalidID) {
				t.Errorf("NewID(machineId=%q): got %v, want ErrInvalidID", machineID, err)
			}
		})
	}
}

func TestNewIDAcceptsMachineIDUpTo256Bytes(t *testing.T) {
	for _, machineID := range []string{"a", "...", "-", strings.Repeat("a", MaxIDPartLen)} {
		if _, err := NewID(TypePi, machineID, "s"); err != nil {
			t.Errorf("NewID(machineId=%q): %v", machineID, err)
		}
	}
}

func TestNewIDRejectsInvalidSessionID(t *testing.T) {
	cases := map[string]string{
		"empty":                "",
		"slash":                "a/b",
		"dot":                  ".",
		"dot dot":              "..",
		"NUL":                  "a\x00b",
		"newline":              "a\nb",
		"tab":                  "a\tb",
		"DEL":                  "a\x7fb",
		"C1 control":           "a\u0085b",
		"invalid UTF-8":        "a\xffb",
		"truncated UTF-8":      "a\xe4\xb8",
		"longer than 256 B":    strings.Repeat("a", MaxIDPartLen+1),
		"multibyte over 256 B": strings.Repeat("é", MaxIDPartLen/2) + "a",
	}
	for name, sessionID := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewID(TypePi, "m", sessionID)
			if !errors.Is(err, ErrInvalidID) {
				t.Errorf("NewID(sessionId=%q): got %v, want ErrInvalidID", sessionID, err)
			}
		})
	}
}

func TestNewIDAcceptsAnyOtherSessionIDUpTo256Bytes(t *testing.T) {
	cases := []string{
		"a:b:c",
		"with space",
		"100%",
		"中文會話",
		"...",
		".hidden",
		`back\slash`,
		strings.Repeat("a", MaxIDPartLen),
		strings.Repeat("é", MaxIDPartLen/2),
	}
	for _, sessionID := range cases {
		if _, err := NewID(TypePi, "m", sessionID); err != nil {
			t.Errorf("NewID(sessionId=%q): %v", sessionID, err)
		}
	}
}

func TestIDsAreCaseSensitive(t *testing.T) {
	a, _ := NewID(TypePi, "Mac", "Session")
	b, _ := NewID(TypePi, "mac", "session")
	if a == b || a.String() == b.String() {
		t.Errorf("%v and %v must be different IDs", a, b)
	}
}

func TestIDStringJoinsPartsWithColons(t *testing.T) {
	id, err := NewID(TypePi, "mac", "s:1")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := id.String(), "pi:mac:s:1"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestParseIDRoundTripsString(t *testing.T) {
	for _, s := range []string{"pi:mac:abc", "pi:mac:a:b:c", "pi:m.1_-:中文 :%"} {
		id, err := ParseID(s)
		if err != nil {
			t.Fatalf("ParseID(%q): %v", s, err)
		}
		if got := id.String(); got != s {
			t.Errorf("ParseID(%q).String() = %q", s, got)
		}
	}
}

func TestParseIDSplitsOnFirstTwoColons(t *testing.T) {
	id, err := ParseID("pi:mac:a:b")
	if err != nil {
		t.Fatal(err)
	}
	want := ID{Type: TypePi, MachineID: "mac", SessionID: "a:b"}
	if id != want {
		t.Errorf("got %+v, want %+v", id, want)
	}
}

func TestParseIDRejectsInvalidString(t *testing.T) {
	for _, s := range []string{"", "pi", "pi:mac", "pi:mac:", "claude:mac:s", "pi::s", "pi:a/b:s", "pi:mac:a/b"} {
		if _, err := ParseID(s); !errors.Is(err, ErrInvalidID) {
			t.Errorf("ParseID(%q): got %v, want ErrInvalidID", s, err)
		}
	}
}

func FuzzParseIDRoundTrip(f *testing.F) {
	for _, s := range []string{"pi:mac:abc", "pi:mac:a:b", "pi::", "x:y:z"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		id, err := ParseID(s)
		if err != nil {
			return
		}
		if got := id.String(); got != s {
			t.Errorf("ParseID(%q).String() = %q", s, got)
		}
		again, err := NewID(id.Type, id.MachineID, id.SessionID)
		if err != nil || again != id {
			t.Errorf("NewID(parts of %q) = %+v, %v", s, again, err)
		}
	})
}
