// Package permission defines tool capability and approval classifications.
package permission

import "fmt"

// FileAccess controls which filesystem operations run without per-call
// approval. Values are persisted as stable lowercase strings.
type FileAccess string

const (
	// FileAccessWorkspaceRead automatically permits reads inside the workspace.
	// Workspace writes and all external access require approval.
	FileAccessWorkspaceRead FileAccess = "workspace_read"
	// FileAccessWorkspaceWrite automatically permits reads and writes inside the
	// workspace. All external access requires approval.
	FileAccessWorkspaceWrite FileAccess = "workspace_write"
	// FileAccessUnrestricted automatically permits all filesystem access.
	FileAccessUnrestricted FileAccess = "unrestricted"
)

// DefaultFileAccess is the least permissive valid filesystem capability.
const DefaultFileAccess = FileAccessWorkspaceRead

// Valid reports whether value is a supported filesystem capability.
func (a FileAccess) Valid() bool {
	switch a {
	case FileAccessWorkspaceRead, FileAccessWorkspaceWrite, FileAccessUnrestricted:
		return true
	default:
		return false
	}
}

// ParseFileAccess converts one persisted value into a valid FileAccess.
func ParseFileAccess(value string) (FileAccess, error) {
	access := FileAccess(value)
	if !access.Valid() {
		return "", fmt.Errorf("invalid file access %q", value)
	}
	return access, nil
}

// ShellAccess controls whether arbitrary shell commands require approval.
// Values are persisted as stable lowercase strings.
type ShellAccess string

const (
	// ShellAccessApprovalRequired requires approval for every shell command.
	ShellAccessApprovalRequired ShellAccess = "approval_required"
	// ShellAccessUnrestricted permits arbitrary shell commands. It grants
	// effective unrestricted filesystem and process access.
	ShellAccessUnrestricted ShellAccess = "unrestricted"
)

// DefaultShellAccess requires approval for all shell commands.
const DefaultShellAccess = ShellAccessApprovalRequired

// Valid reports whether value is a supported shell capability.
func (a ShellAccess) Valid() bool {
	switch a {
	case ShellAccessApprovalRequired, ShellAccessUnrestricted:
		return true
	default:
		return false
	}
}

// ParseShellAccess converts one persisted value into a valid ShellAccess.
func ParseShellAccess(value string) (ShellAccess, error) {
	access := ShellAccess(value)
	if !access.Valid() {
		return "", fmt.Errorf("invalid shell access %q", value)
	}
	return access, nil
}

// Scope identifies whether a resolved filesystem target is inside the session
// workspace or outside it.
type Scope string

const (
	// ScopeWorkspace identifies a target under the resolved workspace root.
	ScopeWorkspace Scope = "workspace"
	// ScopeOutsideWorkspace identifies a target outside the workspace root.
	ScopeOutsideWorkspace Scope = "outside_workspace"
	// ScopeGlobal identifies a command whose filesystem effects cannot be
	// predicted before it runs.
	ScopeGlobal Scope = "global"
)

// Kind identifies the capability requested by a tool operation.
type Kind string

const (
	// KindFileRead identifies a filesystem read.
	KindFileRead Kind = "file_read"
	// KindFileWrite identifies a filesystem mutation.
	KindFileWrite Kind = "file_write"
	// KindShell identifies arbitrary process execution.
	KindShell Kind = "shell"
	// KindMCP identifies a call to an MCP tool that is not declared read-only.
	KindMCP Kind = "mcp"
)

// RequiresApproval reports whether a request of kind at scope must wait for
// approval under the two session access settings.
func RequiresApproval(fileAccess FileAccess, shellAccess ShellAccess, kind Kind, scope Scope) bool {
	if kind == KindShell {
		return shellAccess != ShellAccessUnrestricted
	}
	if fileAccess == FileAccessUnrestricted {
		return false
	}
	if scope != ScopeWorkspace {
		return true
	}
	return kind == KindFileWrite && fileAccess != FileAccessWorkspaceWrite
}
