package logging

import (
	"context"
	"log/slog"
	"time"

	adktrace "github.com/soasurs/adk/trace"
)

// NewADKTracer returns an ADK tracer that records successful runtime details
// at debug and terminal failures at error.
func NewADKTracer(logger *slog.Logger) adktrace.Tracer {
	return &adkTracer{logger: OrDiscard(logger)}
}

type adkTracer struct {
	logger *slog.Logger
}

func (t *adkTracer) Start(ctx context.Context, event adktrace.Event) (context.Context, adktrace.Span) {
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	t.logger.LogAttrs(ctx, slog.LevelDebug, "adk operation started", traceAttrs(ctx, event)...)
	return ctx, &adkSpan{logger: t.logger, kind: event.Kind, startedAt: event.Time}
}

type adkSpan struct {
	logger    *slog.Logger
	kind      adktrace.Kind
	startedAt time.Time
}

func (s *adkSpan) AddEvent(ctx context.Context, event adktrace.Event) {
	if event.Kind == "" {
		event.Kind = s.kind
	}
	s.logger.LogAttrs(ctx, slog.LevelDebug, "adk operation event", traceAttrs(ctx, event)...)
}

func (s *adkSpan) End(ctx context.Context, event adktrace.Event) {
	if event.Kind == "" {
		event.Kind = s.kind
	}
	if event.Duration == 0 && !s.startedAt.IsZero() {
		event.Duration = time.Since(s.startedAt)
	}
	level := slog.LevelDebug
	message := "adk operation completed"
	if event.Err != nil && event.Kind == adktrace.KindToolCall {
		level = slog.LevelError
		message = "adk operation failed"
	} else if event.Err != nil {
		message = "adk operation failed"
	}
	s.logger.LogAttrs(ctx, level, message, traceAttrs(ctx, event)...)
}

func traceAttrs(ctx context.Context, event adktrace.Event) []slog.Attr {
	attrs := make([]slog.Attr, 0, 24)
	addString := func(key, value string) {
		if value != "" {
			attrs = append(attrs, slog.String(key, value))
		}
	}
	addInt := func(key string, value int) {
		if value != 0 {
			attrs = append(attrs, slog.Int(key, value))
		}
	}
	addInt64 := func(key string, value int64) {
		if value != 0 {
			attrs = append(attrs, slog.Int64(key, value))
		}
	}

	addString("request_id", RequestID(ctx))
	addString("kind", string(event.Kind))
	addString("run_id", event.RunID)
	addString("turn_id", event.TurnID)
	addString("session_id", event.SessionID)
	addString("agent", event.AgentName)
	addString("model", event.Model)
	addInt("iteration", event.Iteration)
	if event.Stream {
		attrs = append(attrs, slog.Bool("stream", true))
	}
	addInt64("event_id", event.EventID)
	addString("event_author", event.EventAuthor)
	addString("event_role", string(event.EventRole))
	addInt("event_count", event.EventCount)
	if event.Partial {
		attrs = append(attrs, slog.Bool("partial", true))
	}
	addString("tool", event.ToolName)
	addString("tool_call_id", event.ToolCallID)
	if event.Kind == adktrace.KindToolCall || event.ToolName != "" || event.ToolCallID != "" {
		attrs = append(attrs, slog.Int("tool_index", event.ToolIndex))
	}
	addString("finish_reason", string(event.FinishReason))
	addInt64("prompt_tokens", event.PromptTokens)
	addInt64("completion_tokens", event.CompletionTokens)
	addInt64("total_tokens", event.TotalTokens)
	addInt("partial_responses", event.PartialResponses)
	if event.StoppedEarly {
		attrs = append(attrs, slog.Bool("stopped_early", true))
	}
	addString("tool_outcome", string(event.ToolOutcome))
	if event.Duration > 0 {
		attrs = append(attrs, slog.Duration("duration", event.Duration))
	}
	if event.Err != nil {
		attrs = append(attrs, slog.Any("error", event.Err))
	}
	return attrs
}

var _ adktrace.Tracer = (*adkTracer)(nil)
