package server

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"time"

	sessionparser "github.com/Elven9/pi-agent/go/pkg/session-parser"
	"github.com/gofiber/fiber/v3"

	"github.com/Elven9/jarvis-core/internal/metadata"
)

// readMeta answers the session's parsed metadata as JSON (metaJSON).
// Errors are RFC 9457 problem details.
func (s *Sessions) readMeta(c fiber.Ctx) error {
	id, err := sessionIDFromRequest(c)
	if err != nil {
		return newProblem(fiber.StatusBadRequest, err.Error())
	}
	setLogAttrs(c, slog.String("machine_id", id.MachineID), slog.String("session_id", id.SessionID))

	m, err := s.Metadata.Get(c, id)
	if errors.Is(err, metadata.ErrNotFound) {
		return newProblem(fiber.StatusNotFound, fmt.Sprintf("no metadata for session %s", id))
	}
	if err != nil {
		return internalProblem(fmt.Errorf("read metadata %s: %w", id, err))
	}
	return c.JSON(toMetaJSON(m))
}

// metaJSON is the body of the meta route. Absent values are null; lists and maps are never null.
type metaJSON struct {
	Metadata      metadataJSON `json:"metadata"`
	Metrics       *metricsJSON `json:"metrics"` // null for a session without metrics
	ParserVersion string       `json:"parserVersion"`
}

type metadataJSON struct {
	SessionID     string         `json:"sessionId"`
	CWD           string         `json:"cwd"`
	StartedAt     time.Time      `json:"startedAt"`
	FormatVersion int            `json:"formatVersion"`
	ParentSession *string        `json:"parentSession"`
	AgentVersion  *string        `json:"agentVersion"`
	Tags          *tagsJSON      `json:"tags"` // null without a tags entry; tags [] when cleared
	Name          *string        `json:"name"`
	Models        []modelJSON    `json:"models"`
	Errors        []agentErrJSON `json:"errors"`
}

type tagsJSON struct {
	Tags   []string  `json:"tags"`
	Source string    `json:"source"`
	At     time.Time `json:"at"`
}

type modelJSON struct {
	Provider string    `json:"provider"`
	ModelID  string    `json:"modelId"`
	At       time.Time `json:"at"`
}

type agentErrJSON struct {
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// metricsJSON are the totals; raw samples are not stored, so not answered.
type metricsJSON struct {
	Tokens             sessionparser.Tokens                 `json:"tokens"`
	CostUSD            float64                              `json:"costUsd"`
	Turns              int                                  `json:"turns"`
	Model              sessionparser.ModelMetrics           `json:"model"`
	Tools              map[string]sessionparser.ToolMetrics `json:"tools"`
	Compactions        int                                  `json:"compactions"`
	ContextPeakPercent *float64                             `json:"contextPeakPercent"`
	CacheMisses        []sessionparser.CacheMiss            `json:"cacheMisses"`
	Entries            int                                  `json:"entries"`
	EntryID            string                               `json:"entryId"`
	At                 time.Time                            `json:"at"`
}

func toMetaJSON(m metadata.Meta) metaJSON {
	out := metaJSON{Metadata: toMetadataJSON(m.Metadata), ParserVersion: m.ParserVersion}
	if m.Metrics != nil {
		mt := toMetricsJSON(*m.Metrics)
		out.Metrics = &mt
	}
	return out
}

func toMetadataJSON(md sessionparser.Metadata) metadataJSON {
	out := metadataJSON{
		SessionID:     md.SessionID,
		CWD:           md.CWD,
		StartedAt:     md.StartedAt.UTC(),
		FormatVersion: md.FormatVersion,
		ParentSession: md.ParentSession,
		AgentVersion:  md.AgentVersion,
		Name:          md.Name,
		Models:        make([]modelJSON, 0, len(md.Models)),
		Errors:        make([]agentErrJSON, 0, len(md.Errors)),
	}
	if md.Tags != nil {
		out.Tags = &tagsJSON{Tags: append([]string{}, md.Tags.Tags...), Source: md.Tags.Source, At: md.Tags.At.UTC()}
	}
	for _, mc := range md.Models {
		out.Models = append(out.Models, modelJSON{Provider: mc.Provider, ModelID: mc.ModelID, At: mc.At.UTC()})
	}
	for _, e := range md.Errors {
		out.Errors = append(out.Errors, agentErrJSON{Message: e.Message, At: e.At.UTC()})
	}
	return out
}

func toMetricsJSON(mt sessionparser.Metrics) metricsJSON {
	tools := map[string]sessionparser.ToolMetrics{}
	maps.Copy(tools, mt.Tools)
	return metricsJSON{
		Tokens:             mt.Tokens,
		CostUSD:            mt.CostUSD,
		Turns:              mt.Turns,
		Model:              mt.Model,
		Tools:              tools,
		Compactions:        mt.Compactions,
		ContextPeakPercent: mt.ContextPeakPercent,
		CacheMisses:        append([]sessionparser.CacheMiss{}, mt.CacheMisses...),
		Entries:            mt.Entries,
		EntryID:            mt.EntryID,
		At:                 mt.At.UTC(),
	}
}
