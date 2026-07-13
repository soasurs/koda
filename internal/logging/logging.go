// Package logging provides Koda's process logger and request correlation.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

type requestIDContextKey struct{}

// New constructs a text logger writing diagnostic output to output. An empty
// level uses info.
func New(output io.Writer, level string) (*slog.Logger, error) {
	if output == nil {
		return nil, fmt.Errorf("logging: output must not be nil")
	}
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: parsed})), nil
}

// ParseLevel parses a supported Koda log level. An empty value uses info.
func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logging: unsupported level %q", value)
	}
}

// OrDiscard returns logger or a logger that drops every record when logger is
// nil.
func OrDiscard(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return logger
}

// WithRequestID returns a child context carrying a log-only request
// correlation identifier.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, strings.TrimSpace(id))
}

// RequestID returns the request correlation identifier carried by ctx.
func RequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestIDContextKey{}).(string)
	return value
}
