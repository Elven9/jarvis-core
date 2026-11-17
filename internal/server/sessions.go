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
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"

	"github.com/Elven9/jarvis-core/internal/blob"
	"github.com/Elven9/jarvis-core/internal/metadata"
	"github.com/Elven9/jarvis-core/internal/session"
)

// Sessions serves the session routes under /v1/sessions.
type Sessions struct {
	Blobs    blob.Store
	Sessions session.Store
	Metadata metadata.Store
	Logger   *slog.Logger     // error logs of uploads; slog.Default() when nil
	Now      func() time.Time // upload time; time.Now when nil
}

// maxErrorBody is how much of an error body the pi extension shows.
const maxErrorBody = 200

// Register adds the session routes to app.
func (s *Sessions) Register(app *fiber.App) {
	app.Get("/v1/sessions", s.list)
	app.Put("/v1/sessions/:machineId/:sessionId", s.upload)
	app.Get("/v1/sessions/:machineId/:sessionId/raw", s.readRaw)
	app.Get("/v1/sessions/:machineId/:sessionId/meta", s.readMeta)
}

// readRaw answers the session's current raw bytes as application/x-ndjson.
// Errors are RFC 9457 problem details.
func (s *Sessions) readRaw(c fiber.Ctx) error {
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
		return internalProblem(fmt.Errorf("read %s: %w", id, err))
	}
	rc, err := s.Blobs.Open(c, rec.Blob.Hash)
	if err != nil {
		return internalProblem(fmt.Errorf("read %s: %w", id, err))
	}
	c.Set(fiber.HeaderContentType, "application/x-ndjson")
	// fasthttp closes rc once the body has been written.
	return c.SendStream(rc, int(rec.Blob.Size))
}

// upload stores the body as the session's new content (last write wins) and its
// parsed metadata. Write sequence, without a transaction across the steps:
//
//  1. store the blob;
//  2. parse the stored blob and check its header ID (422 when rejected);
//  3. record the upload (session record and upload history);
//  4. replace the session's metadata.
//
// A rejected upload changes no record; its blob stays. A failed step answers 500 and
// rolls nothing back. Rejections and failures are logged as errors.
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
		return s.uploadFailed(c, id, fmt.Errorf("upload %s: store blob: %w", id, err))
	}
	meta, err := s.parseBlob(c, id, info.Hash)
	if errors.Is(err, metadata.ErrInvalidSession) {
		s.logger().ErrorContext(c, "upload rejected", sessionAttrs(id, err)...)
		return fiber.NewError(fiber.StatusUnprocessableEntity, truncate(err.Error(), maxErrorBody))
	}
	if err != nil {
		return s.uploadFailed(c, id, fmt.Errorf("upload %s: %w", id, err))
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if err := s.Sessions.RecordUpload(c, session.Upload{ID: id, Blob: info, UploadedAt: now()}); err != nil {
		return s.uploadFailed(c, id, fmt.Errorf("upload %s: record upload: %w", id, err))
	}
	if err := s.Metadata.Replace(c, meta); err != nil {
		return s.uploadFailed(c, id, fmt.Errorf("upload %s: store metadata: %w", id, err))
	}
	c.Status(fiber.StatusOK)
	return nil
}

// parseBlob parses the stored blob; see metadata.Parse for the errors.
func (s *Sessions) parseBlob(c fiber.Ctx, id session.ID, hash blob.Hash) (metadata.Meta, error) {
	rc, err := s.Blobs.Open(c, hash)
	if err != nil {
		return metadata.Meta{}, fmt.Errorf("open stored blob: %w", err)
	}
	defer func() { _ = rc.Close() }()
	return metadata.Parse(rc, id)
}

// uploadFailed logs err as an error and returns it, which answers a bare 500.
func (s *Sessions) uploadFailed(c fiber.Ctx, id session.ID, err error) error {
	s.logger().ErrorContext(c, "upload failed", sessionAttrs(id, err)...)
	return err
}

func (s *Sessions) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

func sessionAttrs(id session.ID, err error) []any {
	return []any{slog.String("machine_id", id.MachineID), slog.String("session_id", id.SessionID), slog.String("error", err.Error())}
}

// truncate cuts s to at most n bytes without splitting a UTF-8 character.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
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
