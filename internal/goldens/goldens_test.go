package goldens_test

import (
	"bytes"
	"testing"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"

	"github.com/Elven9/jarvis-core/internal/goldens"
)

// parsed is every group of a golden file.
type parsed struct {
	md        *sessionparser.Metadata
	metrics   *sessionparser.Metrics
	calls     []sessionparser.ToolCall
	subagents []sessionparser.Subagent
	skills    *sessionparser.Skills
	pi        *sessionparser.ProjectInstructions
}

func parseAll(t *testing.T, f goldens.File) parsed {
	t.Helper()
	s, err := sessionparser.Parse(bytes.NewReader(f.Data))
	if err != nil {
		t.Fatalf("%s: Parse: %v", f.Name, err)
	}
	if s.Header.ID == "" {
		t.Errorf("%s: header has no session ID", f.Name)
	}
	var p parsed
	var errs [6]error
	p.md, errs[0] = s.Metadata()
	p.metrics, errs[1] = s.Metrics()
	p.calls, errs[2] = s.ToolCalls()
	p.subagents, errs[3] = s.Subagents()
	p.skills, errs[4] = s.Skills()
	p.pi, errs[5] = s.ProjectInstructions()
	for i, group := range []string{"Metadata", "Metrics", "ToolCalls", "Subagents", "Skills", "ProjectInstructions"} {
		if errs[i] != nil {
			t.Fatalf("%s: %s: %v", f.Name, group, errs[i])
		}
	}
	return p
}

func TestEveryGoldenFileParsesInEveryGroup(t *testing.T) {
	for _, f := range goldens.All() {
		t.Run(f.Name, func(t *testing.T) {
			parseAll(t, f)
		})
	}
}

// The upload, store and route tests rely on the golden set having each part the
// parser reads at least once, and a session without metrics.
func TestGoldenFilesCoverEveryParsedPart(t *testing.T) {
	has := map[string]bool{}
	for _, f := range goldens.All() {
		p := parseAll(t, f)
		md := p.md
		has["metrics"] = has["metrics"] || p.metrics != nil
		has["no metrics"] = has["no metrics"] || p.metrics == nil
		has["name"] = has["name"] || md.Name != nil
		has["tags"] = has["tags"] || (md.Tags != nil && len(md.Tags.Tags) > 0)
		has["agent version"] = has["agent version"] || md.AgentVersion != nil
		has["scenario"] = has["scenario"] || md.Scenario != nil
		has["project"] = has["project"] || md.Project != nil
		for _, c := range p.calls {
			has["codemode intent"] = has["codemode intent"] || (c.Codemode != nil && c.Codemode.Intent != nil && len(c.Codemode.Nested) > 0)
		}
		for _, sa := range p.subagents {
			has["subagent result"] = has["subagent result"] || sa.Result != nil
		}
		has["skill load"] = has["skill load"] || len(p.skills.Used) > 0
		for _, in := range append(append([]sessionparser.Instruction{}, p.pi.Workflows...), p.pi.Docs...) {
			has["project instruction"] = true
			has["instruction read"] = has["instruction read"] || len(in.Reads) > 0
		}
	}
	for _, part := range []string{
		"metrics", "no metrics", "name", "tags", "agent version", "scenario", "project",
		"codemode intent", "subagent result", "skill load", "project instruction", "instruction read",
	} {
		if !has[part] {
			t.Errorf("no golden file has %s", part)
		}
	}
}
