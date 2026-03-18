package agent

import (
	"context"
	"encoding/json"
	"strings"
)

type ToolConfirmationRequest struct {
	ToolName  string
	Summary   string
	Arguments string
}

type ToolConfirmationFunc func(ctx context.Context, request ToolConfirmationRequest) error

type toolConfirmationContextKey struct{}

func WithToolConfirmation(ctx context.Context, confirm ToolConfirmationFunc) context.Context {
	if confirm == nil {
		return ctx
	}
	return context.WithValue(ctx, toolConfirmationContextKey{}, confirm)
}

func toolConfirmationFromContext(ctx context.Context) ToolConfirmationFunc {
	if ctx == nil {
		return nil
	}
	confirm, _ := ctx.Value(toolConfirmationContextKey{}).(ToolConfirmationFunc)
	return confirm
}

func requiresToolConfirmation(toolName string) bool {
	switch toolName {
	case "run_shell", "write_file", "create_file":
		return true
	default:
		return false
	}
}

func safeModeSummary(toolName, arguments string) string {
	switch toolName {
	case "run_shell":
		var input struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(arguments), &input) == nil && strings.TrimSpace(input.Command) != "" {
			return "$ " + strings.TrimSpace(input.Command)
		}
	case "write_file":
		var input struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(arguments), &input) == nil && strings.TrimSpace(input.Path) != "" {
			return "write " + strings.TrimSpace(input.Path)
		}
	case "create_file":
		var input struct {
			Path string `json:"path"`
		}
		if json.Unmarshal([]byte(arguments), &input) == nil && strings.TrimSpace(input.Path) != "" {
			return "create " + strings.TrimSpace(input.Path)
		}
	}

	return strings.TrimSpace(arguments)
}
