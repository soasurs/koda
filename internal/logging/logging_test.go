package logging

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	adktrace "github.com/soasurs/adk/trace"
)

func TestNewFiltersConfiguredLevels(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, "warn")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger.Info("hidden")
	logger.Warn("visible")
	if got := output.String(); strings.Contains(got, "hidden") || !strings.Contains(got, "visible") {
		t.Fatalf("logger output = %q", got)
	}
	if _, err := New(&output, "verbose"); err == nil {
		t.Fatal("New(verbose) error = nil")
	}
}

func TestADKTracerUsesDebugForDetailsAndErrorForToolFailures(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(&output, "debug")
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
