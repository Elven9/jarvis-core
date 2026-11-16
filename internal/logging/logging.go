// Package logging builds the JSON slog logger and connects Fiber's log package to it.
package logging

import (
	"io"
	"log/slog"
)

// New returns a logger that writes JSON records at level and above to w.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
