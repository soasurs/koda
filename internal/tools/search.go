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

// ---- grep_search ----

type grepSearchInput struct {
	Pattern     string `json:"pattern" jsonschema:"Regular expression pattern to search for"`
	Path        string `json:"path,omitempty" jsonschema:"File or directory to search in. Defaults to current directory if omitted"`
	Recursive   bool   `json:"recursive,omitempty" jsonschema:"Search directories recursively. Defaults to true"`
	FilePattern string `json:"file_pattern,omitempty" jsonschema:"Glob pattern to filter files (e.g. '*.go', '*.ts'). Only used when path is a directory"`
}

type grepSearchTool struct{ def tool.Definition }

// NewGrepSearchTool creates a tool that searches file contents with regex.
func NewGrepSearchTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[grepSearchInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("grep_search: build schema: %w", err)
	}
	return &grepSearchTool{tool.Definition{
		Name:        "grep_search",
		Description: "Search file contents using a regular expression pattern. Returns matching lines with file name and line number. Searches recursively by default.",
		InputSchema: schema,
	}}, nil
}

func (t *grepSearchTool) Definition() tool.Definition { return t.def }

func (t *grepSearchTool) Run(_ context.Context, _ string, arguments string) (string, error) {
	var input grepSearchInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("grep_search: parse arguments: %w", err)
	}

	path := "."
	if input.Path != "" {
		path = input.Path
	}

	args := []string{"-n", "--color=never"}
	// recursive by default unless explicitly set to false
	if input.Recursive || path == "." {
		args = append(args, "-r")
	}
	if input.FilePattern != "" {
		args = append(args, "--include="+input.FilePattern)
	}
	args = append(args, input.Pattern, path)

	ctx, cancel := context.WithTimeout(context.Background(), defaultShellTimeout)
	defer cancel()

	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "grep", args...)
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	_ = cmd.Run() // grep exits 1 when no matches; treat as empty result

	output := out.String()
	if output == "" {
		return fmt.Sprintf("No matches found for pattern: %s", input.Pattern), nil
	}

	// Limit output
	lines := strings.Split(output, "\n")
	const maxMatches = 200
	suffix := ""
	if len(lines) > maxMatches {
		lines = lines[:maxMatches]
		suffix = fmt.Sprintf("\n... (showing first %d matches)", maxMatches)
	}
	return strings.Join(lines, "\n") + suffix, nil
}

// ---- find_files ----

type findFilesInput struct {
	Pattern string `json:"pattern" jsonschema:"Glob pattern to match file names (e.g. '*.go', '**/*_test.go')"`
	Path    string `json:"path,omitempty" jsonschema:"Root directory to search from. Defaults to current directory if omitted"`
}

type findFilesTool struct{ def tool.Definition }

// NewFindFilesTool creates a tool that finds files matching a glob pattern.
func NewFindFilesTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[findFilesInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("find_files: build schema: %w", err)
	}
	return &findFilesTool{tool.Definition{
		Name:        "find_files",
		Description: "Find files matching a glob pattern in a directory tree. Returns a list of matching file paths.",
		InputSchema: schema,
	}}, nil
}

func (t *findFilesTool) Definition() tool.Definition { return t.def }

func (t *findFilesTool) Run(_ context.Context, _ string, arguments string) (string, error) {
	var input findFilesInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("find_files: parse arguments: %w", err)
	}

	root := "."
	if input.Path != "" {
		root = input.Path
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultShellTimeout)
	defer cancel()

	// Use find with -iname for basic glob; for ** patterns fall back to find -name
	name := input.Pattern
	args := []string{root, "-name", name, "-not", "-path", "*/.git/*", "-not", "-path", "*/node_modules/*"}

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "find", args...)
	cmd.Stdout = &out

	_ = cmd.Run()

	output := strings.TrimSpace(out.String())
	if output == "" {
		return fmt.Sprintf("No files found matching pattern: %s in %s", input.Pattern, root), nil
	}

	lines := strings.Split(output, "\n")
	const maxFiles = 500
	suffix := ""
	if len(lines) > maxFiles {
		lines = lines[:maxFiles]
		suffix = fmt.Sprintf("\n... (showing first %d results)", maxFiles)
	}
	return strings.Join(lines, "\n") + suffix, nil
}
