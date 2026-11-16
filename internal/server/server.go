// Package server builds the HTTP application.
package server

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// New returns the Fiber app with its middleware. Routes are added by the features.
func New(logger *slog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		CaseSensitive:     true, // IDs and routes are case-sensitive
		StrictRouting:     true, // /v1/sessions/a/b/ is not /v1/sessions/a/b
		StreamRequestBody: true, // the upload route streams bodies with no size cap
		Immutable:         true, // ctx values are safe to keep after the handler
		ErrorHandler:      errorHandler,
	})
	app.Use(accessLog(logger), requestid.New(), recover.New())
	return app
}

// errorHandler answers a *Problem as RFC 9457 problem details, a *fiber.Error
// with its status and message as plain text, and anything else with a bare 500
// that does not leak internal error text.
func errorHandler(c fiber.Ctx, err error) error {
	if p, ok := errors.AsType[*Problem](err); ok {
		return sendProblem(c, p)
	}
	code, msg := fiber.StatusInternalServerError, "Internal Server Error"
	if fe, ok := errors.AsType[*fiber.Error](err); ok {
		code, msg = fe.Code, fe.Message
	}
	return c.Status(code).SendString(msg)
}

type logAttrsKey struct{}

// setLogAttrs adds fields to the request's access log record, e.g. the session ID parts.
func setLogAttrs(c fiber.Ctx, attrs ...slog.Attr) {
	prev, _ := c.Locals(logAttrsKey{}).([]slog.Attr)
	c.Locals(logAttrsKey{}, append(prev, attrs...))
}

// accessLog writes one record per request. It runs the error handler itself,
// so the logged status is the one sent, and returns nil so the error is handled once.
// Request bodies and headers are never logged.
func accessLog(logger *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		if err != nil {
			if herr := c.App().ErrorHandler(c, err); herr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		resp := c.Response()
		attrs := []slog.Attr{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.Int("status", resp.StatusCode()),
			slog.Duration("latency", time.Since(start)),
			slog.String("request_id", requestid.FromContext(c)),
		}
		size := int64(len(resp.Body()))
		if resp.IsBodyStream() {
			size = int64(resp.Header.ContentLength()) // -1 when unknown
		}
		if size >= 0 {
			attrs = append(attrs, slog.Int64("size", size))
		}
		if extra, ok := c.Locals(logAttrsKey{}).([]slog.Attr); ok {
			attrs = append(attrs, extra...)
		}
		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
		}
		logger.LogAttrs(c, slog.LevelInfo, "request", attrs...)
		return nil
	}
}
