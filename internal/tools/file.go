package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/soasurs/adk/tool"
)

// ---- read_file ----

type readFileInput struct {
	Path      string `json:"path" jsonschema:"Absolute or relative path to the file to read"`
	StartLine int    `json:"start_line,omitempty" jsonschema:"First line to read, 1-based inclusive. 0 or omitted means start from line 1"`
	EndLine   int    `json:"end_line,omitempty" jsonschema:"Last line to read, 1-based inclusive. 0 or omitted means read to end of file"`
}

type readFileTool struct{ def tool.Definition }

// NewReadFileTool creates a tool that reads a file with optional line range.
func NewReadFileTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[readFileInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("read_file: build schema: %w", err)
	}
	return &readFileTool{tool.Definition{
		Name:        "read_file",
		Description: "Read the contents of a file. Optionally specify start_line and end_line (1-based) to read a specific range. Lines are returned prefixed with their line number.",
		InputSchema: schema,
	}}, nil
}

func (t *readFileTool) Definition() tool.Definition { return t.def }

func (t *readFileTool) Run(_ context.Context, _ string, arguments string) (string, error) {
	var input readFileInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("read_file: parse arguments: %w", err)
	}

	data, err := os.ReadFile(input.Path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	start, end := 1, len(lines)

	if input.StartLine > 0 {
		start = input.StartLine
	}
	if input.EndLine > 0 && input.EndLine < end {
		end = input.EndLine
	}

	if start < 1 {
		start = 1
	}
	if start > len(lines) {
		return fmt.Sprintf("(file has %d lines; start_line %d is out of range)", len(lines), start), nil
	}
	if end > len(lines) {
		end = len(lines)
	}

	const maxLines = 2000
	if end-start+1 > maxLines {
		end = start + maxLines - 1
	}

	var sb strings.Builder
	for i, line := range lines[start-1 : end] {
		fmt.Fprintf(&sb, "%4d | %s\n", start+i, line)
	}
	if end < len(lines) {
		fmt.Fprintf(&sb, "\n... (%d more lines not shown; use start_line/end_line to paginate)", len(lines)-end)
	}
	return sb.String(), nil
}

// ---- write_file ----

type writeFileInput struct {
	Path    string `json:"path" jsonschema:"Absolute or relative path of the file to write"`
	Content string `json:"content" jsonschema:"Complete content to write to the file"`
}

type writeFileTool struct{ def tool.Definition }

// NewWriteFileTool creates a tool that overwrites a file with new content.
func NewWriteFileTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[writeFileInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("write_file: build schema: %w", err)
	}
	return &writeFileTool{tool.Definition{
		Name:        "write_file",
		Description: "Write content to a file, overwriting any existing content. Creates parent directories if needed.",
		InputSchema: schema,
	}}, nil
}

func (t *writeFileTool) Definition() tool.Definition { return t.def }

func (t *writeFileTool) Run(_ context.Context, _ string, arguments string) (string, error) {
	var input writeFileInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("write_file: parse arguments: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(input.Path), 0o755); err != nil {
		return "", fmt.Errorf("write_file: create parent directories: %w", err)
	}

	if err := os.WriteFile(input.Path, []byte(input.Content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}

	lines := strings.Count(input.Content, "\n") + 1
	return fmt.Sprintf("Successfully wrote %d bytes (%d lines) to %s", len(input.Content), lines, input.Path), nil
}

// ---- create_file ----

type createFileInput struct {
	Path    string `json:"path" jsonschema:"Absolute or relative path of the new file to create"`
	Content string `json:"content" jsonschema:"Content to write into the new file"`
}

type createFileTool struct{ def tool.Definition }

// NewCreateFileTool creates a tool that creates a new file, refusing to overwrite existing ones.
func NewCreateFileTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[createFileInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("create_file: build schema: %w", err)
	}
	return &createFileTool{tool.Definition{
		Name:        "create_file",
		Description: "Create a new file with the given content. Returns an error if the file already exists. Use write_file to overwrite existing files.",
		InputSchema: schema,
	}}, nil
}

func (t *createFileTool) Definition() tool.Definition { return t.def }

func (t *createFileTool) Run(_ context.Context, _ string, arguments string) (string, error) {
	var input createFileInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("create_file: parse arguments: %w", err)
	}

	if _, err := os.Stat(input.Path); err == nil {
		return "", fmt.Errorf("create_file: file already exists: %s", input.Path)
	}

	if err := os.MkdirAll(filepath.Dir(input.Path), 0o755); err != nil {
		return "", fmt.Errorf("create_file: create parent directories: %w", err)
	}

	if err := os.WriteFile(input.Path, []byte(input.Content), 0o644); err != nil {
		return "", fmt.Errorf("create_file: %w", err)
	}

	return fmt.Sprintf("Successfully created %s (%d bytes)", input.Path, len(input.Content)), nil
}

// ---- list_directory ----

type listDirInput struct {
	Path string `json:"path" jsonschema:"Absolute or relative path to the directory to list"`
}

type listDirTool struct{ def tool.Definition }

// NewListDirectoryTool creates a tool that lists directory contents.
func NewListDirectoryTool() (tool.Tool, error) {
	schema, err := jsonschema.ForType(reflect.TypeFor[listDirInput](), &jsonschema.ForOptions{})
	if err != nil {
		return nil, fmt.Errorf("list_directory: build schema: %w", err)
	}
	return &listDirTool{tool.Definition{
		Name:        "list_directory",
		Description: "List the contents of a directory, showing files and subdirectories with their sizes and types.",
		InputSchema: schema,
	}}, nil
}

func (t *listDirTool) Definition() tool.Definition { return t.def }

func (t *listDirTool) Run(_ context.Context, _ string, arguments string) (string, error) {
	var input listDirInput
	if err := json.Unmarshal([]byte(arguments), &input); err != nil {
		return "", fmt.Errorf("list_directory: parse arguments: %w", err)
	}

	entries, err := os.ReadDir(input.Path)
	if err != nil {
		return "", fmt.Errorf("list_directory: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s/\n", filepath.Clean(input.Path))
	for _, e := range entries {
		info, _ := e.Info()
		if e.IsDir() {
			fmt.Fprintf(&sb, "  %s/\n", e.Name())
		} else if info != nil {
			fmt.Fprintf(&sb, "  %-40s  %d bytes\n", e.Name(), info.Size())
		} else {
			fmt.Fprintf(&sb, "  %s\n", e.Name())
		}
	}
	fmt.Fprintf(&sb, "\n%d entries", len(entries))
	return sb.String(), nil
}
