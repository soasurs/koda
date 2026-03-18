package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soasurs/adk/tool"
)

func TestReadFileSupportsRangesAndTruncation(t *testing.T) {
	t.Run("range", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sample.txt")
		if err := os.WriteFile(path, []byte("one\ntwo\nthree"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		reader := mustTool(t, NewReadFileTool)
		output := runTool(t, reader, readFileInput{
			Path:      path,
			StartLine: 2,
			EndLine:   3,
		})

		if !strings.Contains(output, "   2 | two\n") {
			t.Fatalf("output missing line 2:\n%s", output)
		}
		if !strings.Contains(output, "   3 | three\n") {
			t.Fatalf("output missing line 3:\n%s", output)
		}
		if strings.Contains(output, "   1 | one\n") {
			t.Fatalf("output unexpectedly included line 1:\n%s", output)
		}
	})

	t.Run("truncation", func(t *testing.T) {
		lines := make([]string, 0, 2005)
		for i := 0; i < 2005; i++ {
			lines = append(lines, "line")
		}
		path := filepath.Join(t.TempDir(), "large.txt")
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		reader := mustTool(t, NewReadFileTool)
		output := runTool(t, reader, readFileInput{Path: path})
		if !strings.Contains(output, "... (5 more lines not shown; use start_line/end_line to paginate)") {
			t.Fatalf("output missing pagination suffix:\n%s", output)
		}
	})
}

func TestWriteAndCreateFileTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sample.txt")

	writer := mustTool(t, NewWriteFileTool)
	writeOutput := runTool(t, writer, writeFileInput{
		Path:    path,
		Content: "hello\nworld",
	})
	if !strings.Contains(writeOutput, "Successfully wrote") {
		t.Fatalf("write output = %q, want success message", writeOutput)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "hello\nworld" {
		t.Fatalf("file content = %q, want %q", string(data), "hello\nworld")
	}

	creator := mustTool(t, NewCreateFileTool)
	if _, err := creator.Run(context.Background(), "", mustJSON(t, createFileInput{
		Path:    path,
		Content: "new content",
	})); err == nil {
		t.Fatal("create_file existing path error = nil, want error")
	}
}

func TestListDirectoryShowsEntries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "child"), 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	listDir := mustTool(t, NewListDirectoryTool)
	output := runTool(t, listDir, listDirInput{Path: dir})

	if !strings.Contains(output, "file.txt") || !strings.Contains(output, "child/") {
		t.Fatalf("output = %q, want file and directory entries", output)
	}
	if !strings.Contains(output, "\n2 entries") {
		t.Fatalf("output = %q, want entry count", output)
	}
}

func TestSearchToolsReturnMatchesAndMisses(t *testing.T) {
	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	txtFile := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(goFile, []byte("package main\n\nconst greeting = \"hello\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	if err := os.WriteFile(txtFile, []byte("goodbye\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	grepTool := mustTool(t, NewGrepSearchTool)
	matchOutput := runTool(t, grepTool, grepSearchInput{
		Pattern:     "hello",
		Path:        dir,
		Recursive:   true,
		FilePattern: "*.go",
	})
	if !strings.Contains(matchOutput, "main.go:3:const greeting = \"hello\"") {
		t.Fatalf("match output = %q, want grep result", matchOutput)
	}

	missOutput := runTool(t, grepTool, grepSearchInput{
		Pattern:   "missing-value",
		Path:      dir,
		Recursive: true,
	})
	if missOutput != "No matches found for pattern: missing-value" {
		t.Fatalf("miss output = %q, want no-match message", missOutput)
	}

	findTool := mustTool(t, NewFindFilesTool)
	findOutput := runTool(t, findTool, findFilesInput{
		Pattern: "*.go",
		Path:    dir,
	})
	if !strings.Contains(findOutput, goFile) {
		t.Fatalf("find output = %q, want %q", findOutput, goFile)
	}
}

func TestRunShellFormatsSuccessAndFailure(t *testing.T) {
	runShell := mustTool(t, NewRunShellTool)

	successOutput := runTool(t, runShell, runShellInput{Command: "printf hello"})
	if !strings.Contains(successOutput, "$ printf hello\nhello") || !strings.Contains(successOutput, "[exit: 0]") {
		t.Fatalf("success output = %q, want echoed command and exit 0", successOutput)
	}

	failureOutput := runTool(t, runShell, runShellInput{Command: "echo boom >&2; exit 3"})
	if !strings.Contains(failureOutput, "boom\n") || !strings.Contains(failureOutput, "[exit: exit status 3]") {
		t.Fatalf("failure output = %q, want stderr and exit status", failureOutput)
	}
}

func TestRunShellRequiresApprovalWhenConfigured(t *testing.T) {
	runShell := mustTool(t, NewRunShellTool)
	called := false
	ctx := WithShellConfirmation(context.Background(), func(ctx context.Context, request ShellConfirmationRequest) error {
		called = true
		if request.Command != "printf hello" {
			t.Fatalf("request.Command = %q, want %q", request.Command, "printf hello")
		}
		return fmt.Errorf("denied")
	})

	_, err := runShell.Run(ctx, "", mustJSON(t, runShellInput{Command: "printf hello"}))
	if err == nil {
		t.Fatal("Run() error = nil, want safe-mode rejection")
	}
	if !called {
		t.Fatal("confirmation callback was not invoked")
	}
	if !strings.Contains(err.Error(), "confirm execution: denied") {
		t.Fatalf("error = %q, want confirmation failure", err.Error())
	}
}

func TestGitToolsReportStatusAndDiff(t *testing.T) {
	dir := initGitRepo(t)
	path := filepath.Join(dir, "tracked.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	statusTool := mustTool(t, NewGitStatusTool)
	statusOutput := runTool(t, statusTool, gitStatusInput{Path: dir})
	if !strings.Contains(statusOutput, "tracked.txt") {
		t.Fatalf("status output = %q, want tracked file name", statusOutput)
	}

	diffTool := mustTool(t, NewGitDiffTool)
	unstagedOutput := runTool(t, diffTool, gitDiffInput{WorkDir: dir})
	if unstagedOutput != "No unstaged changes." {
		t.Fatalf("unstaged output = %q, want no-change message", unstagedOutput)
	}

	runCmd(t, dir, "git", "add", "tracked.txt")
	stagedOutput := runTool(t, diffTool, gitDiffInput{
		Staged:  true,
		WorkDir: dir,
	})
	if !strings.Contains(stagedOutput, "diff --git") || !strings.Contains(stagedOutput, "tracked.txt") {
		t.Fatalf("staged output = %q, want staged diff", stagedOutput)
	}
}

func mustTool(t *testing.T, constructor func() (tool.Tool, error)) tool.Tool {
	t.Helper()

	tool, err := constructor()
	if err != nil {
		t.Fatalf("constructor error = %v", err)
	}
	return tool
}

func runTool(t *testing.T, tool tool.Tool, input any) string {
	t.Helper()

	output, err := tool.Run(context.Background(), "", mustJSON(t, input))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return output
}

func mustJSON(t *testing.T, input any) string {
	t.Helper()

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(data)
}

func initGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runCmd(t, dir, "git", "init")
	return dir
}

func runCmd(t *testing.T, dir, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s error = %v\n%s", name, strings.Join(args, " "), err, output)
	}
}
