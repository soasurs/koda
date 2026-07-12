package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

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

func TestGitRejectsMutatingSubcommands(t *testing.T) {
	workspace := t.TempDir()
	if output, err := exec.Command("git", "init", "-q", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	tools, err := NewReadOnly(Config{Workdir: workspace})
	if err != nil {
		t.Fatalf("NewReadOnly() error = %v", err)
	}
	var status gitOutput
	runTool(t, toolByName(t, tools, "git"), gitInput{Subcommand: "status", Args: []string{"--short"}}, &status)
	if status.ExitCode != 0 {
		t.Fatalf("git status output = %+v", status)
	}
	callToolError(t, toolByName(t, tools, "git"), gitInput{Subcommand: "commit"})
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

func TestGitTargetsClassifyExternalWorktreeMetadata(t *testing.T) {
	workspace := t.TempDir()
	external := t.TempDir()
	gitDir := filepath.Join(external, "gitdir")
	commonDir := filepath.Join(external, "common")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(gitdir) error = %v", err)
	}
	if err := os.MkdirAll(commonDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(common) error = %v", err)
	}
	relativeGitDir, err := filepath.Rel(workspace, gitDir)
	if err != nil {
		t.Fatalf("Rel(gitdir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".git"), []byte("gitdir: "+relativeGitDir+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.git) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "commondir"), []byte("../common\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(commondir) error = %v", err)
	}
	s, err := newService(Config{Workdir: workspace})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	workdir, err := s.resolver.existing(".")
	if err != nil {
		t.Fatalf("existing(workdir) error = %v", err)
	}
	targets, scope := gitTargets(s.resolver, workdir)
	resolvedGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(gitdir) error = %v", err)
	}
	resolvedCommonDir, err := filepath.EvalSymlinks(commonDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(common) error = %v", err)
	}
	if scope != permission.ScopeOutsideWorkspace || !slices.Contains(absoluteTargets(targets...), resolvedGitDir) ||
		!slices.Contains(absoluteTargets(targets...), resolvedCommonDir) {
		t.Fatalf("gitTargets() = %+v, %q", targets, scope)
	}
}

func TestSearchFindGitAndShellErrorPaths(t *testing.T) {
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

	for _, args := range [][]string{{"--no-index"}, {"--output=result"}, {"--textconv"}, {"-c", "core.pager=cat"}} {
		if _, err := validateGit("diff", args); err == nil {
			t.Fatalf("validateGit(%q) error = nil", args)
		}
	}
	if _, err := validateGit("status", nil); err != nil {
		t.Fatalf("validateGit(status) error = %v", err)
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
