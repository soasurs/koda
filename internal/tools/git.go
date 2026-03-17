package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/soasurs/adk/tool"
)

// ---- git_status ----

type gitStatusInput struct {
	Path string `json:"path,omitempty" jsonschema:"Repository path. Defaults to current directory if omitted"`
}

type gitStatusTool struct{ def tool.Definition }

// NewGitStatusTool creates a tool that shows git working tree status.
func NewGitStatusTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[gitStatusInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("git_status: build schema: %w", err)
	}
	return &gitStatusTool{tool.Definition{
		Name:        "git_status",
		Description: "Show the working tree status of a git repository. Lists staged, unstaged, and untracked files.",
		InputSchema: schema,
	}}, nil
}

func (t *gitStatusTool) Definition() tool.Definition { return t.def }

func (t *gitStatusTool) Run(_ context.Context, _ string, arguments string) (string, error) {
	var input gitStatusInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("git_status: parse arguments: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultShellTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "status")
	if input.Path != "" {
		cmd.Dir = input.Path
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	_ = cmd.Run()
	return strings.TrimSpace(out.String()), nil
}

// ---- git_diff ----

type gitDiffInput struct {
	Staged  bool   `json:"staged,omitempty" jsonschema:"Show staged (indexed) changes instead of unstaged changes"`
	Path    string `json:"path,omitempty" jsonschema:"Limit diff to this file or directory path (optional)"`
	WorkDir string `json:"work_dir,omitempty" jsonschema:"Repository path. Defaults to current directory if omitted"`
}

type gitDiffTool struct{ def tool.Definition }

// NewGitDiffTool creates a tool that shows git diffs.
func NewGitDiffTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[gitDiffInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("git_diff: build schema: %w", err)
	}
	return &gitDiffTool{tool.Definition{
		Name:        "git_diff",
		Description: "Show git diff of changes. Use staged=true to see staged (indexed) changes. Optionally limit to a specific file path.",
		InputSchema: schema,
	}}, nil
}

func (t *gitDiffTool) Definition() tool.Definition { return t.def }

func (t *gitDiffTool) Run(_ context.Context, _ string, arguments string) (string, error) {
	var input gitDiffInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("git_diff: parse arguments: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultShellTimeout)
	defer cancel()

	args := []string{"diff"}
	if input.Staged {
		args = append(args, "--staged")
	}
	if input.Path != "" {
		args = append(args, "--", input.Path)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	if input.WorkDir != "" {
		cmd.Dir = input.WorkDir
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	_ = cmd.Run()

	output := strings.TrimSpace(out.String())
	if output == "" {
		if input.Staged {
			return "No staged changes.", nil
		}
		return "No unstaged changes.", nil
	}

	// Limit diff output
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes] + fmt.Sprintf("\n... (diff truncated at %d KB)", maxOutputBytes/1024)
	}
	return output, nil
}
