package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"
	"github.com/gofiber/fiber/v3"

	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

// readParsed parses the session's current upload with every group of the session parser
// built into the server and answers the result as JSON (parsedJSON). Nothing of it is
// stored: each request parses the raw file again, so it shows what this parser version reads.
// A stored file this parser rejects is a 422 problem naming the parse error.
// Errors are RFC 9457 problem details.
func (s *Sessions) readParsed(c fiber.Ctx) error {
	id, err := sessionIDFromRequest(c)
	if err != nil {
		return newProblem(fiber.StatusBadRequest, err.Error())
	}
	setLogAttrs(c, slog.String("machine_id", id.MachineID), slog.String("session_id", id.SessionID))

	rec, err := s.Sessions.Get(c, id)
	if errors.Is(err, session.ErrNotFound) {
		return newProblem(fiber.StatusNotFound, fmt.Sprintf("no session %s", id))
	}
	if err != nil {
		return internalProblem(fmt.Errorf("parse %s: %w", id, err))
	}
	rc, err := s.Blobs.Open(c, rec.Blob.Hash)
	if err != nil {
		return internalProblem(fmt.Errorf("parse %s: %w", id, err))
	}
	defer func() { _ = rc.Close() }()
	p, err := metadata.ParseAll(rc, id)
	if errors.Is(err, metadata.ErrInvalidSession) {
		return newProblem(fiber.StatusUnprocessableEntity,
			fmt.Sprintf("session %s does not parse with session-parser %s: %v", id, metadata.ParserVersion(), err))
	}
	if err != nil {
		return internalProblem(fmt.Errorf("parse %s: %w", id, err))
	}
	return c.JSON(toParsedJSON(p))
}

// parsedJSON is the body of the parsed session route. Absent values are null; lists and
// maps are never null. JSON values the session holds (tool arguments, details) are as written.
type parsedJSON struct {
	Metadata            parsedMetadataJSON      `json:"metadata"`
	Metrics             *parsedMetricsJSON      `json:"metrics"` // null for a session without metrics
	ToolCalls           []toolCallJSON          `json:"toolCalls"`
	Subagents           []subagentJSON          `json:"subagents"`
	Skills              skillsJSON              `json:"skills"`
	ProjectInstructions projectInstructionsJSON `json:"projectInstructions"`
	ParserVersion       string                  `json:"parserVersion"`
}

// parsedMetadataJSON is the meta route's metadata plus the fields it does not store.
type parsedMetadataJSON struct {
	metadataJSON
	Scenario *string      `json:"scenario"`
	Project  *projectJSON `json:"project"`
}

type projectJSON struct {
	Name   string            `json:"name"`
	Source projectSourceJSON `json:"source"`
	At     time.Time         `json:"at"`
}

// projectSourceJSON has dir for a local project, the other fields for a remote one; the
// fields of the other kind are null.
type projectSourceJSON struct {
	Kind     string  `json:"kind"`
	Dir      *string `json:"dir"`
	Provider *string `json:"provider"`
	Repo     *string `json:"repo"`
	Path     *string `json:"path"`
	Tag      *string `json:"tag"`
	Commit   *string `json:"commit"`
}

// parsedMetricsJSON are the meta route's metrics plus the raw samples.
type parsedMetricsJSON struct {
	metricsJSON
	Samples samplesJSON `json:"samples"`
}

type samplesJSON struct {
	TTFTMs    []float64            `json:"ttftMs"`
	LatencyMs []float64            `json:"latencyMs"`
	ToolMs    map[string][]float64 `json:"toolMs"`
}

type toolCallJSON struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Args      json.RawMessage `json:"args"`
	ArgsError *string         `json:"argsError"`
	EntryID   string          `json:"entryId"`
	CalledAt  time.Time       `json:"calledAt"`
	Ref       *int            `json:"ref"`
	Result    *toolResultJSON `json:"result"`   // null when the call has no result
	Codemode  *codemodeJSON   `json:"codemode"` // null for tools other than codemode
}

type toolResultJSON struct {
	EntryID    string                       `json:"entryId"`
	IsError    bool                         `json:"isError"`
	Content    []sessionparser.ContentBlock `json:"content"`
	Details    json.RawMessage              `json:"details"`
	At         time.Time                    `json:"at"`
	DurationMs *float64                     `json:"durationMs"`
}

type codemodeJSON struct {
	Code           string           `json:"code"`
	Intent         *string          `json:"intent"`
	Nested         []nestedCallJSON `json:"nested"`
	FullOutputPath *string          `json:"fullOutputPath"`
}

type nestedCallJSON struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	ArgsPreview string          `json:"argsPreview"`
	Args        json.RawMessage `json:"args"` // null when the preview was cut
	Status      string          `json:"status"`
	DurationMs  *float64        `json:"durationMs"`
	Error       *string         `json:"error"`
	CostUSD     *float64        `json:"costUsd"`
}

type subagentJSON struct {
	ID            string              `json:"id"`
	Scenario      string              `json:"scenario"`
	Kind          string              `json:"kind"`
	Task          string              `json:"task"`
	Project       *string             `json:"project"`
	RestartOf     *string             `json:"restartOf"`
	StartedBy     string              `json:"startedBy"`
	StartedAt     time.Time           `json:"startedAt"`
	TaskMessageID string              `json:"taskMessageId"`
	EntryID       string              `json:"entryId"`
	Result        *subagentResultJSON `json:"result"` // null without a result
}

type subagentResultJSON struct {
	ResultID         string          `json:"resultId"`
	Status           string          `json:"status"`
	Summary          string          `json:"summary"`
	Reason           *string         `json:"reason"`
	DetailKind       *string         `json:"detailKind"`
	Detail           json.RawMessage `json:"detail"`
	EndedAt          time.Time       `json:"endedAt"`
	ChildSessionFile *string         `json:"childSessionFile"`
	Delivered        bool            `json:"delivered"`
	EntryID          string          `json:"entryId"`
	MessageID        string          `json:"messageId"`
}

type skillsJSON struct {
	Available []skillJSON    `json:"available"`
	Used      []skillUseJSON `json:"used"`
}

type skillJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
}

type skillUseJSON struct {
	Name     string          `json:"name"`
	Location string          `json:"location"`
	Loads    []skillLoadJSON `json:"loads"`
}

type skillLoadJSON struct {
	Via     string    `json:"via"`
	CallID  string    `json:"callId"`
	EntryID string    `json:"entryId"`
	At      time.Time `json:"at"`
	Failed  bool      `json:"failed"`
}

type projectInstructionsJSON struct {
	Workflows []instructionJSON `json:"workflows"`
	Docs      []instructionJSON `json:"docs"`
}

type instructionJSON struct {
	Path  string                `json:"path"`
	When  string                `json:"when"`
	Reads []instructionReadJSON `json:"reads"`
}

type instructionReadJSON struct {
	Tool       string    `json:"tool"`
	InCodemode bool      `json:"inCodemode"`
	CallID     string    `json:"callId"`
	EntryID    string    `json:"entryId"`
	At         time.Time `json:"at"`
	Failed     bool      `json:"failed"`
}

func toParsedJSON(p metadata.Parsed) parsedJSON {
	out := parsedJSON{
		Metadata: parsedMetadataJSON{
			metadataJSON: toMetadataJSON(p.Metadata),
			Scenario:     p.Metadata.Scenario,
			Project:      toProjectJSON(p.Metadata.Project),
		},
		ToolCalls:           make([]toolCallJSON, 0, len(p.ToolCalls)),
		Subagents:           make([]subagentJSON, 0, len(p.Subagents)),
		Skills:              toSkillsJSON(p.Skills),
		ProjectInstructions: projectInstructionsJSON{Workflows: toInstructionsJSON(p.ProjectInstructions.Workflows), Docs: toInstructionsJSON(p.ProjectInstructions.Docs)},
		ParserVersion:       p.ParserVersion,
	}
	if mt := p.Metrics; mt != nil {
		toolMs := map[string][]float64{}
		for name, ms := range mt.Samples.ToolMs {
			toolMs[name] = append([]float64{}, ms...)
		}
		out.Metrics = &parsedMetricsJSON{
			metricsJSON: toMetricsJSON(*mt),
			Samples: samplesJSON{
				TTFTMs:    append([]float64{}, mt.Samples.TTFTMs...),
				LatencyMs: append([]float64{}, mt.Samples.LatencyMs...),
				ToolMs:    toolMs,
			},
		}
	}
	for _, c := range p.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, toToolCallJSON(c))
	}
	for _, sa := range p.Subagents {
		out.Subagents = append(out.Subagents, toSubagentJSON(sa))
	}
	return out
}

func toProjectJSON(p *sessionparser.Project) *projectJSON {
	if p == nil {
		return nil
	}
	src := projectSourceJSON{Kind: p.Source.Kind}
	if p.Source.Kind == sessionparser.ProjectLocal {
		src.Dir = &p.Source.Dir
	} else {
		src.Provider, src.Repo, src.Path, src.Tag, src.Commit = &p.Source.Provider, &p.Source.Repo, &p.Source.Path, &p.Source.Tag, &p.Source.Commit
	}
	return &projectJSON{Name: p.Name, Source: src, At: p.At.UTC()}
}

func toToolCallJSON(c sessionparser.ToolCall) toolCallJSON {
	out := toolCallJSON{
		ID: c.ID, Name: c.Name, Args: c.Args, ArgsError: c.ArgsError,
		EntryID: c.EntryID, CalledAt: c.CalledAt.UTC(), Ref: c.Ref,
	}
	if r := c.Result; r != nil {
		out.Result = &toolResultJSON{
			EntryID: r.EntryID, IsError: r.IsError, Content: append([]sessionparser.ContentBlock{}, r.Content...),
			Details: r.Details, At: r.At.UTC(), DurationMs: r.DurationMs,
		}
	}
	if s := c.Codemode; s != nil {
		cm := &codemodeJSON{Code: s.Code, Intent: s.Intent, Nested: make([]nestedCallJSON, 0, len(s.Nested)), FullOutputPath: s.FullOutputPath}
		for _, n := range s.Nested {
			cm.Nested = append(cm.Nested, nestedCallJSON{
				ID: n.ID, Name: n.Name, ArgsPreview: n.ArgsPreview, Args: n.Args,
				Status: n.Status, DurationMs: n.DurationMs, Error: n.Error, CostUSD: n.CostUSD,
			})
		}
		out.Codemode = cm
	}
	return out
}

func toSubagentJSON(sa sessionparser.Subagent) subagentJSON {
	out := subagentJSON{
		ID: sa.ID, Scenario: sa.Scenario, Kind: sa.Kind, Task: sa.Task, Project: sa.Project,
		RestartOf: sa.RestartOf, StartedBy: sa.StartedBy, StartedAt: sa.StartedAt.UTC(),
		TaskMessageID: sa.TaskMessageID, EntryID: sa.EntryID,
	}
	if r := sa.Result; r != nil {
		out.Result = &subagentResultJSON{
			ResultID: r.ResultID, Status: r.Status, Summary: r.Summary, Reason: r.Reason,
			DetailKind: r.DetailKind, Detail: r.Detail, EndedAt: r.EndedAt.UTC(),
			ChildSessionFile: r.ChildSessionFile, Delivered: r.Delivered, EntryID: r.EntryID, MessageID: r.MessageID,
		}
	}
	return out
}

func toSkillsJSON(s sessionparser.Skills) skillsJSON {
	out := skillsJSON{Available: make([]skillJSON, 0, len(s.Available)), Used: make([]skillUseJSON, 0, len(s.Used))}
	for _, sk := range s.Available {
		out.Available = append(out.Available, skillJSON(sk))
	}
	for _, u := range s.Used {
		use := skillUseJSON{Name: u.Name, Location: u.Location, Loads: make([]skillLoadJSON, 0, len(u.Loads))}
		for _, l := range u.Loads {
			use.Loads = append(use.Loads, skillLoadJSON{Via: l.Via, CallID: l.CallID, EntryID: l.EntryID, At: l.At.UTC(), Failed: l.Failed})
		}
		out.Used = append(out.Used, use)
	}
	return out
}

func toInstructionsJSON(list []sessionparser.Instruction) []instructionJSON {
	out := make([]instructionJSON, 0, len(list))
	for _, in := range list {
		ij := instructionJSON{Path: in.Path, When: in.When, Reads: make([]instructionReadJSON, 0, len(in.Reads))}
		for _, r := range in.Reads {
			ij.Reads = append(ij.Reads, instructionReadJSON{
				Tool: r.Tool, InCodemode: r.InCodemode, CallID: r.CallID, EntryID: r.EntryID, At: r.At.UTC(), Failed: r.Failed,
			})
		}
		out = append(out, ij)
	}
	return out
}
