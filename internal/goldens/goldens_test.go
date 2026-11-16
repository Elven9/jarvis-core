package goldens_test

import (
	"bytes"
	"testing"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/goldens"
)

func TestEveryGoldenFileParses(t *testing.T) {
	for _, f := range goldens.All() {
		t.Run(f.Name, func(t *testing.T) {
			s, err := sessionparser.Parse(bytes.NewReader(f.Data))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if s.Header.ID == "" {
				t.Error("header has no session ID")
			}
			if _, err := s.Metadata(); err != nil {
				t.Errorf("Metadata: %v", err)
			}
			if _, err := s.Metrics(); err != nil {
				t.Errorf("Metrics: %v", err)
			}
		})
	}
}

// The upload and store tests rely on the golden set having each part of the
// metadata at least once, and a session without metrics.
func TestGoldenFilesCoverEveryMetadataPart(t *testing.T) {
	var metrics, noMetrics, name, tags, agentVersion bool
	for _, f := range goldens.All() {
		s, err := sessionparser.Parse(bytes.NewReader(f.Data))
		if err != nil {
			t.Fatalf("%s: Parse: %v", f.Name, err)
		}
		md, err := s.Metadata()
		if err != nil {
			t.Fatalf("%s: Metadata: %v", f.Name, err)
		}
		m, err := s.Metrics()
		if err != nil {
			t.Fatalf("%s: Metrics: %v", f.Name, err)
		}
		metrics = metrics || m != nil
		noMetrics = noMetrics || m == nil
		name = name || md.Name != nil
		tags = tags || (md.Tags != nil && len(md.Tags.Tags) > 0)
		agentVersion = agentVersion || md.AgentVersion != nil
	}
	for part, ok := range map[string]bool{
		"metrics": metrics, "no metrics": noMetrics, "name": name, "tags": tags, "agent version": agentVersion,
	} {
		if !ok {
			t.Errorf("no golden file has %s", part)
		}
	}
}
