package server

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/session"
)

// Sessions serves the raw session routes under /v1/sessions.
type Sessions struct {
	Blobs    blob.Store
	Sessions session.Store
	Now      func() time.Time // upload time; time.Now when nil
}

// Register adds the session routes to app.
func (s *Sessions) Register(app *fiber.App) {
	app.Put("/v1/sessions/:machineId/:sessionId", s.upload)
}

// upload stores the body as the session's new content (last write wins).
// It answers 200 with an empty body, or an error status with a short plain-text
// body, which the pi extension shows to the user.
func (s *Sessions) upload(c fiber.Ctx) error {
	id, err := sessionIDFromRequest(c)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	setLogAttrs(c, slog.String("machine_id", id.MachineID), slog.String("session_id", id.SessionID))

	if err := checkTextContentType(c.Get(fiber.HeaderContentType)); err != nil {
		return fiber.NewError(fiber.StatusUnsupportedMediaType, err.Error())
	}

	// With StreamRequestBody, bodies above the body limit arrive as a stream;
	// smaller ones are buffered and BodyStream is nil. c.Body() enforces the
	// body limit, so it must only be called when there is no stream.
	var body io.Reader
	if stream := c.Request().BodyStream(); stream != nil {
		body = stream
	} else {
		body = bytes.NewReader(c.Body())
	}
	br := bufio.NewReader(body)
	if _, err := br.Peek(1); errors.Is(err, io.EOF) {
		return fiber.NewError(fiber.StatusBadRequest, "empty body: a session upload must not be empty")
	}

	info, err := s.Blobs.Put(c, br)
	if err != nil {
		return fmt.Errorf("upload %s: %w", id, err)
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if err := s.Sessions.RecordUpload(c, session.Upload{ID: id, Blob: info, UploadedAt: now()}); err != nil {
		return fmt.Errorf("upload %s: %w", id, err)
	}
	c.Status(fiber.StatusOK)
	return nil
}

// sessionIDFromRequest reads the ?type= query parameter (pi when missing) and the
// percent-encoded path parameters, decodes them and validates the ID.
func sessionIDFromRequest(c fiber.Ctx) (session.ID, error) {
	typ := session.TypePi
	switch types := c.Request().URI().QueryArgs().PeekMulti("type"); len(types) {
	case 0:
	case 1:
		typ = session.Type(types[0])
	default:
		return session.ID{}, errors.New("type: give at most one type parameter")
	}
	machineID, err := url.PathUnescape(c.Params("machineId"))
	if err != nil {
		return session.ID{}, fmt.Errorf("machineId: bad percent-encoding: %w", err)
	}
	sessionID, err := url.PathUnescape(c.Params("sessionId"))
	if err != nil {
		return session.ID{}, fmt.Errorf("sessionId: bad percent-encoding: %w", err)
	}
	return session.NewID(typ, machineID, sessionID)
}

// checkTextContentType accepts text/* and application/x-ndjson, with any parameters.
func checkTextContentType(header string) error {
	if header == "" {
		return errors.New("missing content-type: send text/* or application/x-ndjson")
	}
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return fmt.Errorf("invalid content-type %q", header)
	}
	if !strings.HasPrefix(mediaType, "text/") && mediaType != "application/x-ndjson" {
		return fmt.Errorf("unsupported content-type %q: send text/* or application/x-ndjson", mediaType)
	}
	return nil
}
