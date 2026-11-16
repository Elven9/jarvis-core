// Package config reads the server configuration from JARVIS_* environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"
)

const prefix = "JARVIS_"

// Variable names.
const (
	EnvAddr            = prefix + "ADDR"
	EnvDataDir         = prefix + "DATA_DIR"
	EnvLogLevel        = prefix + "LOG_LEVEL"
	EnvShutdownTimeout = prefix + "SHUTDOWN_TIMEOUT"
)

var known = []string{EnvAddr, EnvDataDir, EnvLogLevel, EnvShutdownTimeout}

// Config is the effective server configuration.
type Config struct {
	Addr            string        // listen address
	DataDir         string        // SQLite file and blobs
	LogLevel        slog.Level    // minimum level that is logged
	ShutdownTimeout time.Duration // graceful shutdown limit
}

// Default returns the configuration used when no variable is set.
func Default() Config {
	return Config{
		Addr:            ":8080",
		DataDir:         "./data",
		LogLevel:        slog.LevelInfo,
		ShutdownTimeout: 10 * time.Second,
	}
}

// Load builds the configuration from environ, a list of "KEY=value" entries
// (as returned by os.Environ). Unset variables keep their defaults.
// Invalid values and unknown JARVIS_* variables are errors naming the variable.
func Load(environ []string) (Config, error) {
	cfg := Default()
	var errs []error
	for _, kv := range environ {
		key, value, _ := strings.Cut(kv, "=")
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if err := cfg.set(key, value); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) set(key, value string) error {
	if !slices.Contains(known, key) {
		return fmt.Errorf("%s: unknown variable", key)
	}
	if value == "" {
		return fmt.Errorf("%s: must not be empty", key)
	}
	switch key {
	case EnvAddr:
		if _, _, err := net.SplitHostPort(value); err != nil {
			return fmt.Errorf("%s: invalid address %q: %w", key, value, err)
		}
		c.Addr = value
	case EnvDataDir:
		c.DataDir = value
	case EnvLogLevel:
		level, ok := logLevels[value]
		if !ok {
			return fmt.Errorf("%s: %q is not one of debug, info, warn, error", key, value)
		}
		c.LogLevel = level
	case EnvShutdownTimeout:
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("%s: invalid duration %q: %w", key, value, err)
		}
		if d <= 0 {
			return fmt.Errorf("%s: must be positive, got %s", key, value)
		}
		c.ShutdownTimeout = d
	}
	return nil
}

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// LogValue logs the configuration as a group of its settings.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("addr", c.Addr),
		slog.String("data_dir", c.DataDir),
		slog.String("log_level", strings.ToLower(c.LogLevel.String())),
		slog.String("shutdown_timeout", c.ShutdownTimeout.String()),
	)
}
