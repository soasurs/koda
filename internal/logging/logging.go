// Package logging provides Koda's process logger and request correlation.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type requestIDContextKey struct{}

// New constructs a text logger writing diagnostic output to output and returns
// a function that closes any configured log file. An empty level uses info.
// When logFile is non-empty, output is also written to the file. The path
// supports a leading ~/ for the user home directory. Relative paths are
// resolved relative to ~/.koda/.
func New(output io.Writer, level string, logFile string) (*slog.Logger, func() error, error) {
	if output == nil {
		return nil, nil, fmt.Errorf("logging: output must not be nil")
	}
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, nil, err
	}
	closeLog := func() error { return nil }
	if logFile != "" {
		resolved, err := resolveLogPath(logFile)
		if err != nil {
			return nil, nil, fmt.Errorf("logging: resolve log file path: %w", err)
		}
		file, err := openLogFile(resolved)
		if err != nil {
			return nil, nil, fmt.Errorf("logging: open log file: %w", err)
		}
		output = io.MultiWriter(output, file)
		closeLog = file.Close
	}
	return slog.New(slog.NewTextHandler(output, &slog.HandlerOptions{Level: parsed})), closeLog, nil
}

func resolveLogPath(path string) (string, error) {
	if len(path) >= 2 && path[0] == '~' && (path[1] == '/' || path[1] == '\\') {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home directory: %w", err)
		}
		return filepath.Join(home, filepath.FromSlash(strings.ReplaceAll(path[2:], "\\", "/"))), nil
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".koda", path), nil
}

func openLogFile(path string) (io.WriteCloser, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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
