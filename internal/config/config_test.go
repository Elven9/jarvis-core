package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadWithoutVariablesReturnsDefaults(t *testing.T) {
	cfg, err := Load([]string{"HOME=/home/me", "PATH=/bin"})
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Addr: ":8080", DataDir: "./data", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second}
	if cfg != want {
		t.Errorf("Load = %+v, want %+v", cfg, want)
	}
}

func TestLoadReadsEveryVariable(t *testing.T) {
	cfg, err := Load([]string{
		"JARVIS_ADDR=127.0.0.1:9000",
		"JARVIS_DATA_DIR=/data",
		"JARVIS_LOG_LEVEL=debug",
		"JARVIS_SHUTDOWN_TIMEOUT=1m30s",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Addr: "127.0.0.1:9000", DataDir: "/data", LogLevel: slog.LevelDebug, ShutdownTimeout: 90 * time.Second}
	if cfg != want {
		t.Errorf("Load = %+v, want %+v", cfg, want)
	}
}

func TestLoadAcceptsEveryLogLevel(t *testing.T) {
	levels := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}
	for name, want := range levels {
		cfg, err := Load([]string{"JARVIS_LOG_LEVEL=" + name})
		if err != nil || cfg.LogLevel != want {
			t.Errorf("JARVIS_LOG_LEVEL=%s: got %v, %v; want %v", name, cfg.LogLevel, err, want)
		}
	}
}

func TestLoadRejectsInvalidValuesNamingTheVariable(t *testing.T) {
	cases := []string{
		"JARVIS_ADDR=8080",
		"JARVIS_ADDR=",
		"JARVIS_DATA_DIR=",
		"JARVIS_LOG_LEVEL=verbose",
		"JARVIS_LOG_LEVEL=INFO",
		"JARVIS_SHUTDOWN_TIMEOUT=10",
		"JARVIS_SHUTDOWN_TIMEOUT=0s",
		"JARVIS_SHUTDOWN_TIMEOUT=-1s",
	}
	for _, kv := range cases {
		_, err := Load([]string{kv})
		name, _, _ := strings.Cut(kv, "=")
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("Load(%q): got %v, want an error naming %s", kv, err, name)
		}
	}
}

func TestLoadRejectsUnknownJarvisVariable(t *testing.T) {
	_, err := Load([]string{"JARVIS_ADRR=:9000"})
	if err == nil || !strings.Contains(err.Error(), "JARVIS_ADRR") {
		t.Errorf("Load: got %v, want an error naming JARVIS_ADRR", err)
	}
}

func TestLoadReportsEveryInvalidVariable(t *testing.T) {
	_, err := Load([]string{"JARVIS_LOG_LEVEL=loud", "JARVIS_TYPO=1"})
	if err == nil || !strings.Contains(err.Error(), "JARVIS_LOG_LEVEL") || !strings.Contains(err.Error(), "JARVIS_TYPO") {
		t.Errorf("Load: got %v, want an error naming both variables", err)
	}
}
