package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/gofiber/fiber/v3/log"
)

func decode(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var records []map[string]any
	dec := json.NewDecoder(buf)
	for dec.More() {
		var r map[string]any
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("output is not JSON lines: %v", err)
		}
		records = append(records, r)
	}
	return records
}

func TestNewWritesJSONAtOrAboveLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := New(&buf, slog.LevelWarn)
	logger.Info("hidden")
	logger.Warn("shown", "k", "v")
	records := decode(t, &buf)
	if len(records) != 1 || records[0]["msg"] != "shown" || records[0]["level"] != "WARN" || records[0]["k"] != "v" {
		t.Errorf("records = %v, want one WARN record \"shown\" with k=v", records)
	}
}

func TestFiberLoggerWritesJSONThroughSlog(t *testing.T) {
	var buf bytes.Buffer
	l := NewFiberLogger(New(&buf, slog.LevelDebug))
	l.Info("plain ", 1)
	l.Warnf("formatted %d", 2)
	l.Errorw("with fields", "key", "value")
	records := decode(t, &buf)
	want := []struct{ level, msg string }{{"INFO", "plain 1"}, {"WARN", "formatted 2"}, {"ERROR", "with fields"}}
	if len(records) != len(want) {
		t.Fatalf("records = %v, want %d", records, len(want))
	}
	for i, w := range want {
		if records[i]["level"] != w.level || records[i]["msg"] != w.msg {
			t.Errorf("record %d = %v, want level %s msg %q", i, records[i], w.level, w.msg)
		}
	}
	if records[2]["key"] != "value" {
		t.Errorf("Errorw record = %v, want key=value", records[2])
	}
}

func TestFiberLoggerSetLevelDropsLowerLevels(t *testing.T) {
	var buf bytes.Buffer
	l := NewFiberLogger(New(&buf, slog.LevelDebug))
	l.SetLevel(log.LevelWarn)
	l.Info("hidden")
	l.Warn("shown")
	if records := decode(t, &buf); len(records) != 1 || records[0]["msg"] != "shown" {
		t.Errorf("records = %v, want only \"shown\"", records)
	}
}

func TestFiberLoggerFatalLogsAndExits(t *testing.T) {
	var buf bytes.Buffer
	l := NewFiberLogger(New(&buf, slog.LevelInfo))
	code := -1
	l.exit = func(c int) { code = c }
	l.Fatal("boom")
	if records := decode(t, &buf); code != 1 || len(records) != 1 || records[0]["msg"] != "boom" {
		t.Errorf("exit code %d, records %v; want 1 and one \"boom\" record", code, records)
	}
}

func TestFiberLoggerPanicLogsAndPanics(t *testing.T) {
	var buf bytes.Buffer
	l := NewFiberLogger(New(&buf, slog.LevelInfo))
	defer func() {
		if recover() == nil {
			t.Error("Panic did not panic")
		}
		if records := decode(t, &buf); len(records) != 1 || records[0]["msg"] != "boom" {
			t.Errorf("records = %v, want one \"boom\" record", records)
		}
	}()
	l.Panic("boom")
}
