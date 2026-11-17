package metadata_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"testing/iotest"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/goldens"
	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

func id(t *testing.T, sessionID string) session.ID {
	t.Helper()
	sid, err := session.NewID(session.TypePi, "mac", sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return sid
}

const header = `{"type":"session","version":3,"id":"s1","timestamp":"2026-11-16T09:00:00.000Z","cwd":"/tmp"}` + "\n"

func TestParseReturnsMetadataOfEveryGoldenFile(t *testing.T) {
	for _, f := range goldens.All() {
		t.Run(f.Name, func(t *testing.T) {
			sid := goldenID(t, f.Data)
			m, err := metadata.Parse(bytes.NewReader(f.Data), id(t, sid))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if m.ID != id(t, sid) || m.Metadata.SessionID != sid || m.ParserVersion != metadata.ParserVersion() {
				t.Errorf("Parse = ID %v, header ID %q, parser %q; want %q, %q, %q",
					m.ID, m.Metadata.SessionID, m.ParserVersion, id(t, sid), sid, metadata.ParserVersion())
			}
			if (m.Metrics == nil) != (f.Name == "no-metrics") {
				t.Errorf("Metrics = %v, want nil only for no-metrics", m.Metrics)
			}
		})
	}
}

func TestParseRejectsInvalidSessions(t *testing.T) {
	cases := map[string]string{
		"not JSON":               "hello\n",
		"no header":              `{"type":"message","id":"e1","parentId":null,"timestamp":"2026-11-16T09:00:00.000Z"}` + "\n",
		"old format":             strings.Replace(header, `"version":3`, `"version":2`, 1),
		"entry without id":       header + `{"type":"message","parentId":null,"timestamp":"2026-11-16T09:00:00.000Z"}` + "\n",
		"unknown metrics":        header + custom("pi-agent.metrics", `{"version":99}`),
		"unknown tags":           header + custom("pi-agent.tags", `{"version":99,"tags":["a"],"source":"flag"}`),
		"header ID differs":      strings.Replace(header, `"id":"s1"`, `"id":"s2"`, 1),
		"header ID differs case": strings.Replace(header, `"id":"s1"`, `"id":"S1"`, 1),
		"invalid tool calls":     header + orphanResult,
		"unknown project":        header + custom("pi-agent.project", `{"version":99,"name":"p"}`),
	}
	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := metadata.Parse(strings.NewReader(file), id(t, "s1"))
			if !errors.Is(err, metadata.ErrInvalidSession) {
				t.Errorf("Parse: got %v, want ErrInvalidSession", err)
			}
		})
	}
}

func TestParseReadFailureIsNotInvalidSession(t *testing.T) {
	_, err := metadata.Parse(iotest.ErrReader(errors.New("disk gone")), id(t, "s1"))
	if err == nil || errors.Is(err, metadata.ErrInvalidSession) {
		t.Errorf("Parse: got %v, want a read error that is not ErrInvalidSession", err)
	}
}

func TestParseAllReturnsEveryGroupOfEveryGoldenFile(t *testing.T) {
	for _, f := range goldens.All() {
		t.Run(f.Name, func(t *testing.T) {
			sid := goldenID(t, f.Data)
			got, err := metadata.ParseAll(bytes.NewReader(f.Data), id(t, sid))
			if err != nil {
				t.Fatalf("ParseAll: %v", err)
			}
			s, err := sessionparser.Parse(bytes.NewReader(f.Data))
			if err != nil {
				t.Fatal(err)
			}
			md, _ := s.Metadata()
			metrics, _ := s.Metrics()
			calls, _ := s.ToolCalls()
			subagents, _ := s.Subagents()
			skills, _ := s.Skills()
			instructions, _ := s.ProjectInstructions()
			want := metadata.Parsed{
				ID: id(t, sid), Metadata: *md, Metrics: metrics, ToolCalls: calls, Subagents: subagents,
				Skills: *skills, ProjectInstructions: *instructions, ParserVersion: metadata.ParserVersion(),
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("ParseAll differs from what the parser's groups return")
			}
		})
	}
}

func TestParseAllRejectsSessionsWithAnInvalidGroup(t *testing.T) {
	cases := map[string]string{
		"not JSON":            "hello\n",
		"header ID differs":   strings.Replace(header, `"id":"s1"`, `"id":"s2"`, 1),
		"unknown scenario":    header + custom("pi-agent.scenario", `{"version":99,"name":"demo"}`),
		"unknown metrics":     header + custom("pi-agent.metrics", `{"version":99}`),
		"result without call": header + orphanResult,
	}
	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := metadata.ParseAll(strings.NewReader(file), id(t, "s1"))
			if !errors.Is(err, metadata.ErrInvalidSession) {
				t.Errorf("ParseAll: got %v, want ErrInvalidSession", err)
			}
		})
	}
}

func TestParseAllReadFailureIsNotInvalidSession(t *testing.T) {
	_, err := metadata.ParseAll(iotest.ErrReader(errors.New("disk gone")), id(t, "s1"))
	if err == nil || errors.Is(err, metadata.ErrInvalidSession) {
		t.Errorf("ParseAll: got %v, want a read error that is not ErrInvalidSession", err)
	}
}

// orphanResult is a tool result whose call is not on the branch: Metadata and Metrics accept
// it, ToolCalls does not.
const orphanResult = `{"type":"message","id":"e1","parentId":null,"timestamp":"2026-11-16T09:00:01.000Z","message":{"role":"toolResult","toolCallId":"c1","toolName":"read","content":[],"isError":false}}` + "\n"

func TestParserVersionIsTheLibraryVersionInGoMod(t *testing.T) {
	gomod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^\s*github\.com/Elven9/pi-agent/go (\S+)`).FindSubmatch(gomod)
	if m == nil {
		t.Fatal("go.mod does not require the session-parser module")
	}
	if got := metadata.ParserVersion(); got != string(m[1]) {
		t.Errorf("ParserVersion() = %q, want %q from go.mod", got, m[1])
	}
}

func custom(customType, data string) string {
	return `{"type":"custom","customType":"` + customType + `","id":"e1","parentId":null,"timestamp":"2026-11-16T09:00:01.000Z","data":` + data + "}\n"
}

// goldenID is the session ID in a golden file's header.
func goldenID(t *testing.T, data []byte) string {
	t.Helper()
	m := regexp.MustCompile(`"id":"([^"]+)"`).FindSubmatch(data)
	if m == nil {
		t.Fatal("golden file has no header id")
	}
	return string(m[1])
}
