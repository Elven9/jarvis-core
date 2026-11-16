// Command jarvis runs the jarvis-core HTTP server.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"

	"github.com/Elven9/jarvis-core/internal/config"
	"github.com/Elven9/jarvis-core/internal/logging"
	"github.com/Elven9/jarvis-core/internal/server"
	"github.com/Elven9/jarvis-core/internal/sqlite"
)

// dbFile is the SQLite file inside the data directory.
const dbFile = "jarvis.db"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Environ(), os.Stdout)
	stop()
	if err != nil {
		os.Exit(1)
	}
}

// run serves until ctx is done, then shuts down gracefully.
// Every line it writes to stdout is a JSON log record, including startup errors.
func run(ctx context.Context, environ []string, stdout io.Writer) error {
	cfg, err := config.Load(environ)
	if err != nil {
		logging.New(stdout, slog.LevelInfo).Error("invalid configuration", "error", err.Error())
		return err
	}
	logger := logging.New(stdout, cfg.LogLevel)
	slog.SetDefault(logger)
	log.SetLogger(logging.NewFiberLogger(logger))
	logger.Info("starting", "config", cfg)

	// The schema is migrated before the server listens; any error stops startup.
	db, err := sqlite.Open(ctx, filepath.Join(cfg.DataDir, dbFile), logger)
	if err != nil {
		logger.Error("cannot open database", "error", err.Error())
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("close database", "error", err.Error())
		}
	}()

	app := server.New(logger)
	err = app.Listen(cfg.Addr, fiber.ListenConfig{
		DisableStartupMessage: true,
		ListenerNetwork:       fiber.NetworkTCP, // IPv4 and IPv6; Fiber defaults to IPv4 only
		GracefulContext:       ctx,
		ShutdownTimeout:       cfg.ShutdownTimeout,
		ListenerAddrFunc: func(addr net.Addr) {
			logger.Info("listening", "addr", addr.String())
		},
	})
	if err != nil {
		logger.Error("server stopped", "error", err.Error())
		return fmt.Errorf("serve: %w", err)
	}
	logger.Info("stopped")
	return nil
}
