package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/permission"
)

func TestReadAndEditFileUseHashlineRevision(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "example.go")
	if err := os.WriteFile(path, []byte("package example\n\nfunc Value() int {\n\treturn 1\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	tools, err := NewBuild(Config{Workdir: workspace, FileAccess: permission.FileAccessWorkspaceWrite})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}

	var read readFileOutput
	runTool(t, toolByName(t, tools, "read_file"), readFileInput{Path: "example.go"}, &read)
	if read.Revision == "" || len(read.Lines) != 5 || read.Lines[3].Content != "\treturn 1" {
		t.Fatalf("read_file output = %+v", read)
	}

	var edited editFileOutput
	runTool(t, toolByName(t, tools, "edit_file"), editFileInput{
		Path:             "example.go",
		ExpectedRevision: read.Revision,
		Edits: []editOperation{{
			Operation: "replace",
			Start:     read.Lines[3].Anchor,
			Content:   "\treturn 2",
		}},
	}, &edited)
	if edited.Revision == read.Revision || len(edited.FileChanges) != 1 || len(edited.FileChanges[0].Hunks) != 1 {
		t.Fatalf("edit_file output = %+v", edited)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(contents), "package example\n\nfunc Value() int {\n\treturn 2\n}\n"; got != want {
		t.Fatalf("edited contents = %q, want %q", got, want)
	}

	callToolError(t, toolByName(t, tools, "edit_file"), editFileInput{
		Path:             "example.go",
		ExpectedRevision: read.Revision,
		Edits: []editOperation{{
			Operation: "replace",
			Start:     read.Lines[3].Anchor,
			Content:   "\treturn 3",
		}},
	})
	contents, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(after stale edit) error = %v", err)
	}
	if string(contents) != "package example\n\nfunc Value() int {\n\treturn 2\n}\n" {
		t.Fatalf("stale edit changed file: %q", contents)
	}
}

func TestFileAccessClassifiesExternalPathsAndSymlinks(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	externalPath := filepath.Join(external, "secret.txt")
	if err := os.WriteFile(externalPath, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(external) error = %v", err)
	}
	resolvedExternalPath, err := filepath.EvalSymlinks(externalPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(external path) error = %v", err)
	}
	if err := os.Symlink(externalPath, filepath.Join(workspace, "link.txt")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	authorizer := newRecordingAuthorizer()
	tools, err := NewReadOnly(Config{
		Workdir:    workspace,
		FileAccess: permission.FileAccessWorkspaceRead,
		Authorizer: authorizer,
	})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	var output readFileOutput
	runTool(t, toolByName(t, tools, "read_file"), readFileInput{Path: "link.txt"}, &output)
	requests := authorizer.requests()
	if len(requests) != 1 || requests[0].Kind != permission.KindFileRead || requests[0].Scope != permission.ScopeOutsideWorkspace ||
		!slices.Contains(requests[0].TargetPaths, resolvedExternalPath) {
		t.Fatalf("approval requests = %+v", requests)
	}
}

func TestBuildToolsRequireApprovalForWorkspaceWritesAndShell(t *testing.T) {
	workspace := t.TempDir()
	authorizer := newRecordingAuthorizer()
	tools, err := NewBuild(Config{
		Workdir:     workspace,
		FileAccess:  permission.FileAccessWorkspaceRead,
		ShellAccess: permission.ShellAccessApprovalRequired,
		Authorizer:  authorizer,
	})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}
	var created fileWriteOutput
	runTool(t, toolByName(t, tools, "create_file"), createFileInput{Path: "new.txt", Content: "hello\n"}, &created)
	var shell runShellOutput
	runTool(t, toolByName(t, tools, "run_shell"), runShellInput{Command: "printf done"}, &shell)
	if shell.Stdout != "done" || shell.ExitCode != 0 {
		t.Fatalf("run_shell output = %+v", shell)
	}
	requests := authorizer.requests()
	if len(requests) != 2 || requests[0].Kind != permission.KindFileWrite || requests[0].Scope != permission.ScopeWorkspace ||
		requests[1].Kind != permission.KindShell || requests[1].Scope != permission.ScopeGlobal {
		t.Fatalf("approval requests = %+v", requests)
	}
}

func TestRunShellTimeoutKillsChildProcesses(t *testing.T) {
	workspace := t.TempDir()
	values, err := NewBuild(Config{
		Workdir: workspace, ShellAccess: permission.ShellAccessUnrestricted,
	})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}
	callToolError(t, toolByName(t, values, "run_shell"), runShellInput{
		Command: "sleep 30 & echo $! > child.pid; wait", TimeoutSeconds: 1,
	})
	encodedPID, err := os.ReadFile(filepath.Join(workspace, "child.pid"))
	if err != nil {
		t.Fatalf("ReadFile(child.pid) error = %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(encodedPID)))
	if err != nil {
		t.Fatalf("Atoi(child PID) error = %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	for deadline := time.Now().Add(time.Second); ; {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived shell timeout: %v", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestApprovalPreservesProviderToolCallMetadata(t *testing.T) {
	workspace := t.TempDir()
	authorizer := newRecordingAuthorizer()
	values, err := NewBuild(Config{
		Workdir:     workspace,
		FileAccess:  permission.FileAccessWorkspaceRead,
		ShellAccess: permission.ShellAccessApprovalRequired,
		Authorizer:  authorizer,
	})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}
	arguments := json.RawMessage(`{"path":"new.txt","content":"hello"}`)
	result, err := toolByName(t, values, "create_file").Run(t.Context(), tool.Call{
		ID:        "call-1",
		Name:      "create_file",
		Arguments: arguments,
	})
	if err != nil || result == nil {
		t.Fatalf("create_file.Run() = %v, %v", result, err)
	}
	requests := authorizer.requests()
	if len(requests) != 1 || requests[0].ToolCallID != "call-1" || requests[0].ToolName != "create_file" ||
		string(requests[0].Arguments) != string(arguments) {
		t.Fatalf("approval requests = %+v", requests)
	}
	arguments[0] = 'x'
	if string(requests[0].Arguments) != `{"path":"new.txt","content":"hello"}` {
		t.Fatalf("approval arguments were not cloned: %q", requests[0].Arguments)
	}
}

func TestSearchAndFindFilesExposeEditableMetadata(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}
	workspace := t.TempDir()
	rgConfig := filepath.Join(t.TempDir(), "ripgreprc")
	if err := os.WriteFile(rgConfig, []byte("--glob=!*.go\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(ripgreprc) error = %v", err)
	}
	t.Setenv("RIPGREP_CONFIG_PATH", rgConfig)
	if err := os.WriteFile(filepath.Join(workspace, "sample.go"), []byte("package sample\n\nfunc Target() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	tools, err := NewReadOnly(Config{Workdir: workspace})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	var searched searchTextOutput
	runTool(t, toolByName(t, tools, "search_text"), searchTextInput{Pattern: "Target"}, &searched)
	if len(searched.Matches) != 1 || searched.Matches[0].Anchor == "" || searched.Matches[0].Revision == "" {
		t.Fatalf("search_text output = %+v", searched)
	}
	var found findFilesOutput
	runTool(t, toolByName(t, tools, "find_files"), findFilesInput{Globs: []string{"**/*.go"}}, &found)
	if !slices.Contains(found.Files, "sample.go") {
		t.Fatalf("find_files output = %+v", found)
	}
}

func TestSearchGlobSchemasDescribeJSONArrayInputs(t *testing.T) {
	values, err := NewReadOnly(Config{Workdir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	for _, test := range []struct {
		name    string
		example string
	}{
		{name: "search_text", example: `["**/*.go"]`},
		{name: "find_files", example: `["**/*_test.go"]`},
	} {
		definition := toolByName(t, values, test.name).Definition()
		if definition.InputSchema == nil || definition.InputSchema.Properties["globs"] == nil {
			t.Fatalf("%s globs schema is missing", test.name)
		}
		description := definition.InputSchema.Properties["globs"].Description
		if !strings.Contains(description, test.example) {
			t.Fatalf("%s globs description = %q; want JSON array example %q", test.name, description, test.example)
		}
	}
}

func TestPlanRunShellAllowsOnlyReadOnlyGit(t *testing.T) {
	repository := t.TempDir()
	command := exec.Command("git", "init", "-q", repository)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init error = %v; output = %s", err, output)
	}
	workspace := filepath.Join(repository, "nested")
	if err := os.Mkdir(workspace, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	authorizer := newRecordingAuthorizer()
	values, err := NewReadOnly(Config{Workdir: workspace, FileAccess: permission.FileAccessWorkspaceRead, Authorizer: authorizer})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	var output runShellOutput
	runTool(t, toolByName(t, values, "run_shell"), runShellInput{Command: "git status --short"}, &output)
	if output.ExitCode != 0 {
		t.Fatalf("git status output = %+v", output)
	}
	requests := authorizer.requests()
	realRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if len(requests) != 1 || requests[0].Kind != permission.KindFileRead || requests[0].Scope != permission.ScopeOutsideWorkspace ||
		!slices.Contains(requests[0].TargetPaths, realRepository) || !slices.Contains(requests[0].TargetPaths, filepath.Join(realRepository, ".git")) {
		t.Fatalf("approval requests = %+v", requests)
	}
	callToolError(t, toolByName(t, values, "run_shell"), runShellInput{Command: "rm -rf ."})
	callToolError(t, toolByName(t, values, "run_shell"), runShellInput{Command: "git commit -m nope"})
	callToolError(t, toolByName(t, values, "run_shell"), runShellInput{Command: "git branch new-branch"})
	runTool(t, toolByName(t, values, "run_shell"), runShellInput{Command: `git log --format='%h %s' -1`}, &output)

	if err := os.WriteFile(filepath.Join(repository, ".gitattributes"), []byte("*.txt filter=evil\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(.gitattributes) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(tracked.txt) error = %v", err)
	}
	for _, args := range [][]string{
		{"-C", repository, "-c", "user.name=Test", "-c", "user.email=test@example.com", "add", "."},
		{"-C", repository, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "initial"},
		{"-C", repository, "config", "filter.evil.clean", "sh -c 'echo ran > filter-ran; cat'"},
	} {
		if result, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v error = %v; output = %s", args, err, result)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "tracked.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(modified tracked.txt) error = %v", err)
	}
	runTool(t, toolByName(t, values, "run_shell"), runShellInput{Command: "git status --short"}, &output)
	if _, err := os.Stat(filepath.Join(repository, "filter-ran")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Git filter executed in Plan mode: %v", err)
	}
}

func TestWholeFileChangeShowsTrailingNewlineOnlyChange(t *testing.T) {
	change := wholeFileChange("example.txt", parseTextFile("hello"), parseTextFile("hello\n"), FileChangeUpdate)
	if len(change.Hunks) != 1 || len(change.Hunks[0].Lines) != 2 ||
		change.Hunks[0].Lines[0].Kind != DiffLineRemoved || change.Hunks[0].Lines[1].Kind != DiffLineAdded {
		t.Fatalf("wholeFileChange() = %+v", change)
	}
}

func TestWriteFileReapprovesWhenContentsChangeDuringApproval(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "example.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	authorizer := &callbackAuthorizer{callback: func(index int, _ Approval) error {
		if index == 0 {
			return os.WriteFile(path, []byte("concurrent\n"), 0o600)
		}
		return nil
	}}
	values, err := NewBuild(Config{Workdir: workspace, FileAccess: permission.FileAccessWorkspaceRead, Authorizer: authorizer})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}
	var output fileWriteOutput
	runTool(t, toolByName(t, values, "write_file"), writeFileInput{Path: "example.txt", Content: "new\n"}, &output)
	if len(authorizer.values) != 2 {
		t.Fatalf("approval count = %d, want 2", len(authorizer.values))
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "new\n" {
		t.Fatalf("ReadFile() = %q, %v", got, err)
	}
}

func TestWriteFileAndListDirectory(t *testing.T) {
	workspace := t.TempDir()
	existingPath := filepath.Join(workspace, "existing.txt")
	if err := os.WriteFile(existingPath, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".hidden"), []byte("hidden\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(hidden) error = %v", err)
	}
	tools, err := NewBuild(Config{Workdir: workspace, FileAccess: permission.FileAccessWorkspaceWrite})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}

	var read readFileOutput
	runTool(t, toolByName(t, tools, "read_file"), readFileInput{Path: "existing.txt"}, &read)
	var written fileWriteOutput
	runTool(t, toolByName(t, tools, "write_file"), writeFileInput{
		Path:             "existing.txt",
		Content:          "new\n",
		ExpectedRevision: read.Revision,
	}, &written)
	if written.Revision == read.Revision || len(written.FileChanges) != 1 || written.FileChanges[0].Kind != FileChangeUpdate {
		t.Fatalf("write_file output = %+v", written)
	}
	info, err := os.Stat(existingPath)
	if err != nil {
		t.Fatalf("Stat(existing) error = %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("existing mode = %o, want 600", info.Mode().Perm())
	}
	callToolError(t, toolByName(t, tools, "write_file"), writeFileInput{
		Path:             "existing.txt",
		Content:          "stale\n",
		ExpectedRevision: read.Revision,
	})

	var created fileWriteOutput
	runTool(t, toolByName(t, tools, "write_file"), writeFileInput{Path: "nested/new.txt", Content: "created\n"}, &created)
	if len(created.FileChanges) != 1 || created.FileChanges[0].Kind != FileChangeCreate {
		t.Fatalf("new write output = %+v", created)
	}

	var listed listDirectoryOutput
	runTool(t, toolByName(t, tools, "list_directory"), listDirectoryInput{Path: ".", MaxEntries: 1}, &listed)
	if len(listed.Entries) != 1 || !listed.Truncated || listed.Entries[0].Name == ".hidden" {
		t.Fatalf("list_directory limited output = %+v", listed)
	}
	runTool(t, toolByName(t, tools, "list_directory"), listDirectoryInput{Path: ".", IncludeHidden: true}, &listed)
	if !slices.ContainsFunc(listed.Entries, func(entry directoryEntry) bool { return entry.Name == ".hidden" }) {
		t.Fatalf("list_directory hidden output = %+v", listed)
	}
}

func TestEditFileSupportsInsertDeleteAndRejectsOverlap(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "example.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\ndelta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	tools, err := NewBuild(Config{Workdir: workspace, FileAccess: permission.FileAccessWorkspaceWrite})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}
	var read readFileOutput
	runTool(t, toolByName(t, tools, "read_file"), readFileInput{Path: "example.txt"}, &read)
	var edited editFileOutput
	runTool(t, toolByName(t, tools, "edit_file"), editFileInput{
		Path:             "example.txt",
		ExpectedRevision: read.Revision,
		Edits: []editOperation{
			{Operation: "insert_before", Anchor: read.Lines[1].Anchor, Content: "before"},
			{Operation: "replace", Start: read.Lines[2].Anchor, Content: "G"},
			{Operation: "delete", Start: read.Lines[3].Anchor},
		},
	}, &edited)
	if len(edited.Anchors) == 0 || len(edited.FileChanges[0].Hunks) != 3 {
		t.Fatalf("edit_file output = %+v", edited)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(contents), "alpha\nbefore\nbeta\nG\n"; got != want {
		t.Fatalf("edited contents = %q, want %q", got, want)
	}

	file := parseTextFile("one\ntwo\n")
	first, _ := file.anchor(1)
	if _, err := validateEdits(file, []editOperation{
		{Operation: "delete", Start: first},
		{Operation: "replace", Start: first, Content: "ONE"},
	}); err == nil {
		t.Fatal("validateEdits(overlap) error = nil")
	}
	if _, err := validateEdits(file, []editOperation{{Operation: "unknown", Start: first}}); err == nil {
		t.Fatal("validateEdits(unknown) error = nil")
	}
}

func TestHashlineFindBestMatchAndContentHash(t *testing.T) {
	file := parseTextFile("alpha\nbeta\nalpha\ngamma\n")

	anchor1, err := file.anchor(1)
	if err != nil {
		t.Fatalf("anchor(1) error = %v", err)
	}
	line, err := file.verifyAnchor(anchor1)
	if err != nil {
		t.Fatalf("verifyAnchor(exact) error = %v", err)
	}
	if line != 1 {
		t.Fatalf("verifyAnchor(exact) = %d, want 1", line)
	}

	anchor3, err := file.anchor(3)
	if err != nil {
		t.Fatalf("anchor(3) error = %v", err)
	}
	line, err = file.verifyAnchor(anchor3)
	if err != nil {
		t.Fatalf("verifyAnchor(duplicate) error = %v", err)
	}
	if line != 3 {
		t.Fatalf("verifyAnchor(duplicate) = %d, want 3", line)
	}

	edited := parseTextFile("// header\nalpha\nbeta\nalpha\ngamma\n")
	line, err = edited.verifyAnchor(anchor1)
	if err != nil {
		t.Fatalf("verifyAnchor(shifted) error = %v", err)
	}
	if line != 2 {
		t.Fatalf("verifyAnchor(shifted) = %d, want 2", line)
	}

	missing, _ := file.anchor(4)
	truncated := parseTextFile("// header\nalpha\nbeta\nalpha\n")
	_, err = truncated.verifyAnchor(missing)
	if err == nil {
		t.Fatal("verifyAnchor(truncated) error = nil, want stale anchor")
	}
}

func TestPathsAndApprovalFailures(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(workspace, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	s, err := newService(Config{Workdir: workspace})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	resolved, err := s.resolver.writeTarget("linked/new/file.txt")
	if err != nil {
		t.Fatalf("writeTarget() error = %v", err)
	}
	if resolved.scope != permission.ScopeOutsideWorkspace || resolved.exists {
		t.Fatalf("writeTarget() = %+v", resolved)
	}
	if _, err := NewReadOnly(Config{Workdir: workspace, FileAccess: permission.FileAccess("invalid")}); err == nil {
		t.Fatal("NewReadOnly(invalid access) error = nil")
	}
	if _, err := NewReadOnly(Config{Workdir: filepath.Join(workspace, "missing")}); err == nil {
		t.Fatal("NewReadOnly(missing workdir) error = nil")
	}

	tools, err := NewBuild(Config{Workdir: workspace, FileAccess: permission.FileAccessWorkspaceRead})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}
	callToolError(t, toolByName(t, tools, "create_file"), createFileInput{Path: "denied.txt", Content: "no"})
	if err := os.WriteFile(filepath.Join(workspace, "text.txt"), []byte("value\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	callToolError(t, toolByName(t, tools, "read_file"), readFileInput{Path: "text.txt", StartLine: -1})
}

func TestReadFileTruncationAndTextValidation(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "long.txt"), []byte("first line\nsecond line\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(long) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "binary.bin"), []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}
	tools, err := NewReadOnly(Config{Workdir: workspace})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	var read readFileOutput
	runTool(t, toolByName(t, tools, "read_file"), readFileInput{Path: "long.txt", MaxChars: 8}, &read)
	if len(read.Lines) != 1 || !read.Truncated || read.NextLine != 2 {
		t.Fatalf("truncated read output = %+v", read)
	}
	callToolError(t, toolByName(t, tools, "read_file"), readFileInput{Path: "binary.bin"})
	callToolError(t, toolByName(t, tools, "read_file"), readFileInput{Path: "."})
}

func TestSearchFindAndShellErrorPaths(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg is not installed")
	}
	workspace := t.TempDir()
	for _, name := range []string{"first.go", "second.go"} {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte("needle\nneedle\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	tools, err := NewBuild(Config{Workdir: workspace, FileAccess: permission.FileAccessWorkspaceWrite, ShellAccess: permission.ShellAccessUnrestricted})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}
	var searched searchTextOutput
	runTool(t, toolByName(t, tools, "search_text"), searchTextInput{Pattern: "needle", MaxResults: 1}, &searched)
	if len(searched.Matches) != 1 || !searched.Truncated {
		t.Fatalf("limited search output = %+v", searched)
	}
	runTool(t, toolByName(t, tools, "search_text"), searchTextInput{Pattern: "absent"}, &searched)
	if len(searched.Matches) != 0 || searched.Truncated {
		t.Fatalf("empty search output = %+v", searched)
	}
	callToolError(t, toolByName(t, tools, "search_text"), searchTextInput{})

	var found findFilesOutput
	runTool(t, toolByName(t, tools, "find_files"), findFilesInput{Globs: []string{"**/*.go"}, MaxResults: 1}, &found)
	if len(found.Files) != 1 || !found.Truncated {
		t.Fatalf("limited find output = %+v", found)
	}

	buffer := newTruncatingBuffer(3)
	if _, err := buffer.Write([]byte("abcdef")); err != nil {
		t.Fatalf("truncating buffer Write() error = %v", err)
	}
	if !buffer.truncated || buffer.String() == "abc" {
		t.Fatalf("truncating buffer = %q, truncated = %t", buffer.String(), buffer.truncated)
	}

	var shell runShellOutput
	runTool(t, toolByName(t, tools, "run_shell"), runShellInput{Command: "printf error >&2; exit 3"}, &shell)
	if shell.ExitCode != 3 || shell.Stderr != "error" {
		t.Fatalf("run_shell failure output = %+v", shell)
	}
}

type recordingAuthorizer struct {
	values []Approval
}

type callbackAuthorizer struct {
	values   []Approval
	callback func(int, Approval) error
}

func (a *callbackAuthorizer) Authorize(_ context.Context, approval Approval) error {
	index := len(a.values)
	a.values = append(a.values, approval)
	return a.callback(index, approval)
}

func newRecordingAuthorizer() *recordingAuthorizer {
	return new(recordingAuthorizer)
}

func (a *recordingAuthorizer) Authorize(_ context.Context, approval Approval) error {
	a.values = append(a.values, approval)
	return nil
}

func (a *recordingAuthorizer) requests() []Approval {
	return append([]Approval(nil), a.values...)
}

func toolByName(t *testing.T, tools []tool.Tool, name string) tool.Tool {
	t.Helper()
	for _, candidate := range tools {
		if candidate.Definition().Name == name {
			return candidate
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func runTool(t *testing.T, candidate tool.Tool, input, output any) {
	t.Helper()
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	result, err := candidate.Run(t.Context(), tool.Call{Name: candidate.Definition().Name, Arguments: arguments})
	if err != nil {
		t.Fatalf("%s.Run() error = %v", candidate.Definition().Name, err)
	}
	if err := json.Unmarshal(result.StructuredContent, output); err != nil {
		t.Fatalf("Unmarshal(%s result) error = %v; result = %s", candidate.Definition().Name, err, result.StructuredContent)
	}
}

func callToolError(t *testing.T, candidate tool.Tool, input any) {
	t.Helper()
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	_, err = candidate.Run(t.Context(), tool.Call{Name: candidate.Definition().Name, Arguments: arguments})
	if err == nil {
		t.Fatalf("%s.Run() error = nil, want handled error", candidate.Definition().Name)
	}
	var handledError *tool.HandledError
	if !errors.As(err, &handledError) {
		t.Fatalf("%s.Run() error = %v, want handled error", candidate.Definition().Name, err)
	}
}

func newTestWebFetchTool(t *testing.T) tool.Tool {
	t.Helper()
	tools, err := NewReadOnly(Config{Workdir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	return toolByName(t, tools, "web_fetch")
}

func TestValidateFetchURL(t *testing.T) {
	tests := []struct {
		name   string
		rawURL string
		ok     bool
	}{
		{name: "https url", rawURL: "https://example.com", ok: true},
		{name: "http url", rawURL: "http://example.com/path?query=1", ok: true},
		{name: "ftp scheme rejected", rawURL: "ftp://example.com/file"},
		{name: "file scheme rejected", rawURL: "file:///etc/passwd"},
		{name: "no host", rawURL: "https://"},
		{name: "loopback ipv4", rawURL: "http://127.0.0.1:8080/api"},
		{name: "loopback ipv6", rawURL: "http://[::1]:8080/api"},
		{name: "private ipv4 10.x", rawURL: "http://10.0.0.1/api"},
		{name: "private ipv4 172.16.x", rawURL: "http://172.16.0.1/api"},
		{name: "private ipv4 192.168.x", rawURL: "http://192.168.1.1/api"},
		{name: "link local", rawURL: "http://169.254.1.1/api"},
		{name: "localhost", rawURL: "http://localhost:3000/api"},
		{name: "localhost with uppercase", rawURL: "http://LOCALHOST/api"},
		{name: "dot local", rawURL: "http://internal.local/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.rawURL)
			if err != nil {
				t.Fatalf("url.Parse() error = %v", err)
			}
			err = validateFetchURL(u, false)
			if tt.ok && err != nil {
				t.Fatalf("validateFetchURL() error = %v, want nil", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("validateFetchURL() error = nil, want error")
			}
		})
	}
}

func TestWebFetchEmptyURL(t *testing.T) {
	webFetch := newTestWebFetchTool(t)
	callToolError(t, webFetch, webFetchInput{URL: ""})
	callToolError(t, webFetch, webFetchInput{URL: "   "})
}

func TestWebFetchRestrictedURL(t *testing.T) {
	webFetch := newTestWebFetchTool(t)
	callToolError(t, webFetch, webFetchInput{URL: "http://127.0.0.1/api"})
	callToolError(t, webFetch, webFetchInput{URL: "http://localhost/test"})
	callToolError(t, webFetch, webFetchInput{URL: "file:///etc/passwd"})
}

func TestWebFetchSuccess(t *testing.T) {
	testAllowLoopback = true
	defer func() { testAllowLoopback = false }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") == "" {
			t.Error("Accept header is missing")
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	webFetch := newTestWebFetchTool(t)
	var output webFetchOutput
	runTool(t, webFetch, webFetchInput{URL: server.URL + "/api"}, &output)

	if output.StatusCode != http.StatusOK {
		t.Fatalf("status_code = %d, want %d", output.StatusCode, http.StatusOK)
	}
	if output.Content != "hello world" {
		t.Fatalf("content = %q, want %q", output.Content, "hello world")
	}
	if output.URL != server.URL+"/api" {
		t.Fatalf("url = %q, want %q", output.URL, server.URL+"/api")
	}
}

func TestWebFetchMarkdownAcceptHeader(t *testing.T) {
	testAllowLoopback = true
	defer func() { testAllowLoopback = false }()
	var acceptHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptHeader = r.Header.Get("Accept")
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	webFetch := newTestWebFetchTool(t)
	runTool(t, webFetch, webFetchInput{URL: server.URL}, &webFetchOutput{})

	if !strings.HasPrefix(acceptHeader, "text/markdown") {
		t.Fatalf("Accept header = %q, want text/markdown prefix", acceptHeader)
	}
}

func TestWebFetchTruncation(t *testing.T) {
	testAllowLoopback = true
	defer func() { testAllowLoopback = false }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("a", 1000)))
	}))
	defer server.Close()

	webFetch := newTestWebFetchTool(t)
	var output webFetchOutput
	runTool(t, webFetch, webFetchInput{URL: server.URL, MaxChars: 10}, &output)

	if output.Truncated != true {
		t.Fatal("truncated = false, want true")
	}
	if len(output.Content) != 10 {
		t.Fatalf("len(content) = %d, want 10", len(output.Content))
	}
}

func TestWebFetchNoTruncation(t *testing.T) {
	testAllowLoopback = true
	defer func() { testAllowLoopback = false }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("short"))
	}))
	defer server.Close()

	webFetch := newTestWebFetchTool(t)
	var output webFetchOutput
	runTool(t, webFetch, webFetchInput{URL: server.URL}, &output)

	if output.Truncated {
		t.Fatal("truncated = true, want false")
	}
}

func TestWebFetchRedirectToRestricted(t *testing.T) {
	testAllowLoopback = true
	defer func() { testAllowLoopback = false }()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/restricted", http.StatusFound)
	}))
	defer server.Close()

	webFetch := newTestWebFetchTool(t)
	callToolError(t, webFetch, webFetchInput{URL: server.URL + "/redirect"})
}
