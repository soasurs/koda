package logging

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adktrace "github.com/soasurs/adk/trace"
)

func TestNewWritesToLogFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "koda.log")
	var output bytes.Buffer
	logger, err := New(&output, "info", logPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Info("hello")
	if !strings.Contains(output.String(), "hello") {
		t.Fatalf("buffer output = %q, want hello", output.String())
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "hello") {
		t.Fatalf("file output = %q, want hello", string(content))
	}
}

func TestNewCreatesLogFileParentDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	logPath := filepath.Join(dir, "koda.log")
	var output bytes.Buffer
	logger, err := New(&output, "info", logPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Info("created")
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
}

func TestNewRejectsNilOutput(t *testing.T) {
	if _, err := New(nil, "", ""); err == nil {
		t.Fatal("New(nil) error = nil")
	}
}

func TestResolveLogPathExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}
	got, err := resolveLogPath("~/koda.log")
	if err != nil {
		t.Fatalf("resolveLogPath() error = %v", err)
	}
	if got != filepath.Join(home, "koda.log") {
		t.Fatalf("resolveLogPath(~) = %q, want %q", got, filepath.Join(home, "koda.log"))
	}
}

func TestResolveLogPathRelativeToDotKoda(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}
	got, err := resolveLogPath("koda.log")
	if err != nil {
		t.Fatalf("resolveLogPath() error = %v", err)
	}
	if got != filepath.Join(home, ".koda", "koda.log") {
		t.Fatalf("resolveLogPath(relative) = %q, want %q", got, filepath.Join(home, ".koda", "koda.log"))
	}
}

func TestResolveLogPathKeepsAbsolutePath(t *testing.T) {
	abs := "/var/log/koda.log"
	got, err := resolveLogPath(abs)
	if err != nil || got != abs {
		t.Fatalf("resolveLogPath(absolute) = %q, %v", got, err)
	}
}

func TestNewFiltersConfiguredLevels(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, "warn", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Info("hidden")
	logger.Warn("visible")
	if got := output.String(); strings.Contains(got, "hidden") || !strings.Contains(got, "visible") {
		t.Fatalf("logger output = %q", got)
	}
	if _, err := New(&output, "verbose", ""); err == nil {
		t.Fatal("New(verbose) error = nil")
	}
}

func TestADKTracerUsesDebugForDetailsAndErrorForToolFailures(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, "debug", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx := WithRequestID(context.Background(), "request-1")
	tracer := NewADKTracer(logger)
	ctx, span := tracer.Start(ctx, adktrace.Event{
		Kind:       adktrace.KindToolCall,
		RunID:      "run-1",
		SessionID:  "session-1",
		ToolName:   "read_file",
		Attributes: map[string]any{"arguments": "secret argument"},
	})
	span.End(ctx, adktrace.Event{
		Kind:      adktrace.KindToolCall,
		RunID:     "run-1",
		SessionID: "session-1",
		ToolName:  "read_file",
		Err:       errors.New("tool failed"),
	})
	got := output.String()
	for _, want := range []string{"level=DEBUG", "level=ERROR", "request_id=request-1", "run_id=run-1", "tool=read_file", "tool_index=0", `error="tool failed"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("logger output %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "secret argument") {
		t.Fatalf("logger output contains trace attributes: %q", got)
	}
}
