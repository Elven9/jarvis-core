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
	"syscall"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/log"

	"github.com/Elven9/jarvis-core/internal/config"
	"github.com/Elven9/jarvis-core/internal/logging"
	"github.com/Elven9/jarvis-core/internal/server"
)

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
