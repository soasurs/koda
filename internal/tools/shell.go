package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/soasurs/adk/tool"
)

const defaultShellTimeout = 60 * time.Second
const maxOutputBytes = 50 * 1024 // 50 KB

type ShellConfirmationRequest struct {
	Command string
	WorkDir string
}

type ShellConfirmationFunc func(ctx context.Context, request ShellConfirmationRequest) error

type shellConfirmationContextKey struct{}

type runShellInput struct {
	Command string `json:"command" jsonschema:"Shell command to execute"`
	WorkDir string `json:"work_dir,omitempty" jsonschema:"Working directory for the command. Defaults to the current working directory if omitted"`
}

type runShellTool struct{ def tool.Definition }

// NewRunShellTool creates a tool that executes shell commands.
func NewRunShellTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[runShellInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("run_shell: build schema: %w", err)
	}
	return &runShellTool{tool.Definition{
		Name:        "run_shell",
		Description: "Execute a shell command and return its combined stdout/stderr output. The command runs with a 60-second timeout. Use work_dir to set the working directory.",
		InputSchema: schema,
	}}, nil
}

func (t *runShellTool) Definition() tool.Definition { return t.def }

func WithShellConfirmation(ctx context.Context, confirm ShellConfirmationFunc) context.Context {
	if confirm == nil {
		return ctx
	}
	return context.WithValue(ctx, shellConfirmationContextKey{}, confirm)
}

func shellConfirmationFromContext(ctx context.Context) ShellConfirmationFunc {
	if ctx == nil {
		return nil
	}
	confirm, _ := ctx.Value(shellConfirmationContextKey{}).(ShellConfirmationFunc)
	return confirm
}

func (t *runShellTool) Run(ctx context.Context, _ string, arguments string) (string, error) {
	var input runShellInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("run_shell: parse arguments: %w", err)
	}
	return runShellCommand(ctx, input)
}

func runShellCommand(ctx context.Context, input runShellInput) (string, error) {
	if confirm := shellConfirmationFromContext(ctx); confirm != nil {
		if err := confirm(ctx, ShellConfirmationRequest{
			Command: input.Command,
			WorkDir: input.WorkDir,
		}); err != nil {
			return "", fmt.Errorf("run_shell: confirm execution: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, defaultShellTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)
	if input.WorkDir != "" {
		cmd.Dir = input.WorkDir
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()

	output := out.String()
	// Truncate huge outputs to avoid swamping the LLM context
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + fmt.Sprintf("\n\n... (output truncated at %d KB)", maxOutputBytes/1024)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("$ %s\n", input.Command))
	if output != "" {
		sb.WriteString(output)
		if !strings.HasSuffix(output, "\n") {
			sb.WriteByte('\n')
		}
	}

	if runErr != nil {
		if ctx.Err() != nil {
			sb.WriteString(fmt.Sprintf("[timed out after %s]\n", defaultShellTimeout))
		} else {
			sb.WriteString(fmt.Sprintf("[exit: %s]\n", runErr))
		}
	} else {
		sb.WriteString("[exit: 0]\n")
	}

	return sb.String(), nil
}
