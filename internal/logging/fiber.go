package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/gofiber/fiber/v3/log"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// slog levels for Fiber's extra levels.
const (
	levelTrace = slog.LevelDebug - 4
	levelFatal = slog.LevelError + 4
	levelPanic = slog.LevelError + 8
)

// FiberLogger sends Fiber's log package to slog. Install it with log.SetLogger.
type FiberLogger struct {
	logger *slog.Logger
	min    *slog.LevelVar // Fiber's own level filter (SetLevel), on top of the handler's
	exit   func(code int)
}

var _ log.AllLogger[*slog.Logger] = (*FiberLogger)(nil)

// NewFiberLogger wraps logger for Fiber.
func NewFiberLogger(logger *slog.Logger) *FiberLogger {
	min := new(slog.LevelVar)
	min.Set(levelTrace)
	return &FiberLogger{logger: logger, min: min, exit: os.Exit}
}

func (l *FiberLogger) log(level slog.Level, msg string, args ...any) {
	if level < l.min.Level() {
		return
	}
	l.logger.Log(context.Background(), level, msg, args...)
}

func (l *FiberLogger) fatal(msg string, args ...any) {
	l.log(levelFatal, msg, args...)
	l.exit(1)
}

func (l *FiberLogger) panic(msg string, args ...any) {
	l.log(levelPanic, msg, args...)
	panic(msg)
}

// Trace logs at a level below debug.
func (l *FiberLogger) Trace(v ...any) { l.log(levelTrace, fmt.Sprint(v...)) }

// Debug logs at debug level.
func (l *FiberLogger) Debug(v ...any) { l.log(slog.LevelDebug, fmt.Sprint(v...)) }

// Info logs at info level.
func (l *FiberLogger) Info(v ...any) { l.log(slog.LevelInfo, fmt.Sprint(v...)) }

// Warn logs at warn level.
func (l *FiberLogger) Warn(v ...any) { l.log(slog.LevelWarn, fmt.Sprint(v...)) }

// Error logs at error level.
func (l *FiberLogger) Error(v ...any) { l.log(slog.LevelError, fmt.Sprint(v...)) }

// Fatal logs and exits the process with status 1.
func (l *FiberLogger) Fatal(v ...any) { l.fatal(fmt.Sprint(v...)) }

// Panic logs and panics.
func (l *FiberLogger) Panic(v ...any) { l.panic(fmt.Sprint(v...)) }

// Tracef logs a formatted message at a level below debug.
func (l *FiberLogger) Tracef(format string, v ...any) { l.log(levelTrace, fmt.Sprintf(format, v...)) }

// Debugf logs a formatted message at debug level.
func (l *FiberLogger) Debugf(format string, v ...any) {
	l.log(slog.LevelDebug, fmt.Sprintf(format, v...))
}

// Infof logs a formatted message at info level.
func (l *FiberLogger) Infof(format string, v ...any) {
	l.log(slog.LevelInfo, fmt.Sprintf(format, v...))
}

// Warnf logs a formatted message at warn level.
func (l *FiberLogger) Warnf(format string, v ...any) {
	l.log(slog.LevelWarn, fmt.Sprintf(format, v...))
}

// Errorf logs a formatted message at error level.
func (l *FiberLogger) Errorf(format string, v ...any) {
	l.log(slog.LevelError, fmt.Sprintf(format, v...))
}

// Fatalf logs a formatted message and exits the process with status 1.
func (l *FiberLogger) Fatalf(format string, v ...any) { l.fatal(fmt.Sprintf(format, v...)) }

// Panicf logs a formatted message and panics.
func (l *FiberLogger) Panicf(format string, v ...any) { l.panic(fmt.Sprintf(format, v...)) }

// Tracew logs a message with key-value pairs at a level below debug.
func (l *FiberLogger) Tracew(msg string, kv ...any) { l.log(levelTrace, msg, kv...) }

// Debugw logs a message with key-value pairs at debug level.
func (l *FiberLogger) Debugw(msg string, kv ...any) { l.log(slog.LevelDebug, msg, kv...) }

// Infow logs a message with key-value pairs at info level.
func (l *FiberLogger) Infow(msg string, kv ...any) { l.log(slog.LevelInfo, msg, kv...) }

// Warnw logs a message with key-value pairs at warn level.
func (l *FiberLogger) Warnw(msg string, kv ...any) { l.log(slog.LevelWarn, msg, kv...) }

// Errorw logs a message with key-value pairs at error level.
func (l *FiberLogger) Errorw(msg string, kv ...any) { l.log(slog.LevelError, msg, kv...) }

// Fatalw logs a message with key-value pairs and exits the process with status 1.
func (l *FiberLogger) Fatalw(msg string, kv ...any) { l.fatal(msg, kv...) }

// Panicw logs a message with key-value pairs and panics.
func (l *FiberLogger) Panicw(msg string, kv ...any) { l.panic(msg, kv...) }

// SetLevel drops messages below level.
func (l *FiberLogger) SetLevel(level log.Level) {
	levels := map[log.Level]slog.Level{
		log.LevelTrace: levelTrace,
		log.LevelDebug: slog.LevelDebug,
		log.LevelInfo:  slog.LevelInfo,
		log.LevelWarn:  slog.LevelWarn,
		log.LevelError: slog.LevelError,
		log.LevelFatal: levelFatal,
		log.LevelPanic: levelPanic,
	}
	if s, ok := levels[level]; ok {
		l.min.Set(s)
	}
}

// SetOutput sends the records to w as JSON, keeping the levels.
func (l *FiberLogger) SetOutput(w io.Writer) {
	l.logger = slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: l.min}))
}

// Logger returns the underlying slog logger.
func (l *FiberLogger) Logger() *slog.Logger { return l.logger }

// WithContext returns a logger that adds the request ID of ctx, if it has one.
func (l *FiberLogger) WithContext(ctx any) log.CommonLogger {
	id := requestid.FromContext(ctx)
	if id == "" {
		return l
	}
	return &FiberLogger{logger: l.logger.With("request_id", id), min: l.min, exit: l.exit}
}
