package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soasurs/koda/internal/permission"
)

func TestLoadInstructions(t *testing.T) {
	workspace := t.TempDir()

	subDir := filepath.Join(workspace, "sub")
	if err := os.Mkdir(subDir, 0o700); err != nil {
		t.Fatal(err)
	}

	subAgents := filepath.Join(subDir, "AGENTS.md")
	if err := os.WriteFile(subAgents, []byte("sub rule"), 0o600); err != nil {
		t.Fatal(err)
	}

	svc, err := newService(Config{
		Workdir:    workspace,
		FileAccess: permission.FileAccessWorkspaceRead,
	})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}

	output, err := svc.loadInstructions(t.Context(), loadInstructionsInput{Path: "sub"})
	if err != nil {
		t.Fatalf("loadInstructions(sub) error = %v", err)
	}
	if output.Path != "sub" {
		t.Fatalf("path = %q, want %q", output.Path, "sub")
	}
	if !strings.Contains(output.Content, "sub rule") {
		t.Fatalf("content = %q, want sub rule", output.Content)
	}
	if !strings.Contains(output.Content, "## Instructions from \"sub\"") {
		t.Fatalf("content = %q, want formatted header", output.Content)
	}
}

func TestLoadInstructionsEmptyPath(t *testing.T) {
	svc, err := newService(Config{
		Workdir:    t.TempDir(),
		FileAccess: permission.FileAccessWorkspaceRead,
	})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	_, err = svc.loadInstructions(t.Context(), loadInstructionsInput{Path: ""})
	if err == nil {
		t.Fatal("loadInstructions(\"\") error = nil")
	}
}

func TestLoadInstructionsNotADirectory(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := newService(Config{
		Workdir:    workspace,
		FileAccess: permission.FileAccessWorkspaceRead,
	})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	_, err = svc.loadInstructions(t.Context(), loadInstructionsInput{Path: "file.txt"})
	if err == nil {
		t.Fatal("loadInstructions(file) error = nil")
	}
}

func TestLoadInstructionsWorkspaceRootRejected(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("root rule"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := newService(Config{
		Workdir:    workspace,
		FileAccess: permission.FileAccessWorkspaceRead,
	})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	_, err = svc.loadInstructions(t.Context(), loadInstructionsInput{Path: "."})
	if err == nil {
		t.Fatal("loadInstructions(.) error = nil")
	}
}

func TestLoadInstructionsOutsideWorkspace(t *testing.T) {
	svc, err := newService(Config{
		Workdir:    t.TempDir(),
		FileAccess: permission.FileAccessWorkspaceRead,
	})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	_, err = svc.loadInstructions(t.Context(), loadInstructionsInput{Path: os.TempDir()})
	if err == nil {
		t.Fatal("loadInstructions(outside) error = nil")
	}
}

func TestLoadInstructionsNoAgentsMD(t *testing.T) {
	workspace := t.TempDir()
	subDir := filepath.Join(workspace, "empty")
	if err := os.Mkdir(subDir, 0o700); err != nil {
		t.Fatal(err)
	}
	svc, err := newService(Config{
		Workdir:    workspace,
		FileAccess: permission.FileAccessWorkspaceRead,
	})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	_, err = svc.loadInstructions(t.Context(), loadInstructionsInput{Path: "empty"})
	if err == nil {
		t.Fatal("loadInstructions(no AGENTS.md) error = nil")
	}
	if !strings.Contains(err.Error(), "no AGENTS.md found") {
		t.Fatalf("error = %v, want no AGENTS.md found", err)
	}
}

func TestLoadInstructionsEmptyAgentsMD(t *testing.T) {
	workspace := t.TempDir()
	subDir := filepath.Join(workspace, "sub")
	if err := os.Mkdir(subDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "AGENTS.md"), []byte("  \n  "), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := newService(Config{
		Workdir:    workspace,
		FileAccess: permission.FileAccessWorkspaceRead,
	})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	_, err = svc.loadInstructions(t.Context(), loadInstructionsInput{Path: "sub"})
	if err == nil {
		t.Fatal("loadInstructions(empty AGENTS.md) error = nil")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("error = %v, want is empty", err)
	}
}

func TestLoadInstructionsResolvesSymlink(t *testing.T) {
	workspace := t.TempDir()
	realDir := filepath.Join(workspace, "real")
	if err := os.Mkdir(realDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "AGENTS.md"), []byte("real rule"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(workspace, "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	svc, err := newService(Config{
		Workdir:    workspace,
		FileAccess: permission.FileAccessWorkspaceRead,
	})
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	output, err := svc.loadInstructions(t.Context(), loadInstructionsInput{Path: "link"})
	if err != nil {
		t.Fatalf("loadInstructions(symlink) error = %v", err)
	}
	if !strings.Contains(output.Content, "real rule") {
		t.Fatalf("content = %q, want real rule", output.Content)
	}
}
