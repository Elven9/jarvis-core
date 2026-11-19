package server

import (
	"fmt"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"
	"github.com/gofiber/fiber/v3"
)

// list answers every session, newest upload first, as JSON (listJSON). No pagination.
// Errors are RFC 9457 problem details.
func (s *Sessions) list(c fiber.Ctx) error {
	summaries, err := s.Metadata.List(c)
	if err != nil {
		return internalProblem(fmt.Errorf("list sessions: %w", err))
	}
	out := listJSON{Sessions: make([]sessionRowJSON, 0, len(summaries))}
	for _, sm := range summaries {
		row := sessionRowJSON{
			MachineID:      sm.ID.MachineID,
			SessionID:      sm.ID.SessionID,
			UploadedAt:     sm.UploadedAt.UTC(),
			LastActivityAt: sm.LastActivityAt.UTC(),
			Name:           sm.Name,
			Tags:           sm.Tags,
			AgentVersion:   sm.AgentVersion,
			Scenario:       sm.Scenario,
			Project:        sm.Project,
		}
		if m := sm.Metrics; m != nil {
			row.Metrics = &rowMetricsJSON{Tokens: m.Tokens, CostUSD: m.CostUSD, Turns: m.Turns}
		}
		out.Sessions = append(out.Sessions, row)
	}
	return c.JSON(out)
}

// listJSON is the body of the list route.
type listJSON struct {
	Sessions []sessionRowJSON `json:"sessions"`
}

// sessionRowJSON is one session in the list. Absent values are null.
type sessionRowJSON struct {
	MachineID  string    `json:"machineId"`
	SessionID  string    `json:"sessionId"`
	UploadedAt time.Time `json:"uploadedAt"` // last accepted upload
	// LastActivityAt is the newest user or assistant message in the session.
	LastActivityAt time.Time       `json:"lastActivityAt"`
	Name           *string         `json:"name"`
	Tags           []string        `json:"tags"` // null without a tags entry; [] when cleared
	AgentVersion   *string         `json:"agentVersion"`
	Scenario       *string         `json:"scenario"`
	Project        *string         `json:"project"` // the project's name
	Metrics        *rowMetricsJSON `json:"metrics"` // null for a session without metrics
}

type rowMetricsJSON struct {
	Tokens  sessionparser.Tokens `json:"tokens"`
	CostUSD float64              `json:"costUsd"`
	Turns   int                  `json:"turns"`
}
