package permission

import "testing"

func TestRequiresApproval(t *testing.T) {
	tests := []struct {
		name        string
		fileAccess  FileAccess
		shellAccess ShellAccess
		kind        Kind
		scope       Scope
		want        bool
	}{
		{"workspace read", FileAccessWorkspaceRead, ShellAccessApprovalRequired, KindFileRead, ScopeWorkspace, false},
		{"workspace write restricted", FileAccessWorkspaceRead, ShellAccessApprovalRequired, KindFileWrite, ScopeWorkspace, true},
		{"workspace write allowed", FileAccessWorkspaceWrite, ShellAccessApprovalRequired, KindFileWrite, ScopeWorkspace, false},
		{"external read restricted", FileAccessWorkspaceWrite, ShellAccessApprovalRequired, KindFileRead, ScopeOutsideWorkspace, true},
		{"external write unrestricted", FileAccessUnrestricted, ShellAccessApprovalRequired, KindFileWrite, ScopeOutsideWorkspace, false},
		{"shell requires approval", FileAccessUnrestricted, ShellAccessApprovalRequired, KindShell, ScopeGlobal, true},
		{"shell unrestricted", FileAccessWorkspaceRead, ShellAccessUnrestricted, KindShell, ScopeGlobal, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RequiresApproval(test.fileAccess, test.shellAccess, test.kind, test.scope); got != test.want {
				t.Fatalf("RequiresApproval() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestAccessValuesParseAndValidate(t *testing.T) {
	for _, access := range []FileAccess{
		FileAccessWorkspaceRead,
		FileAccessWorkspaceWrite,
		FileAccessUnrestricted,
	} {
		if !access.Valid() {
			t.Fatalf("FileAccess(%q).Valid() = false", access)
		}
		parsed, err := ParseFileAccess(string(access))
		if err != nil || parsed != access {
			t.Fatalf("ParseFileAccess(%q) = %q, %v", access, parsed, err)
		}
	}
	if FileAccess("invalid").Valid() {
		t.Fatal("invalid FileAccess.Valid() = true")
	}
	if _, err := ParseFileAccess("invalid"); err == nil {
		t.Fatal("ParseFileAccess(invalid) error = nil")
	}

	for _, access := range []ShellAccess{
		ShellAccessApprovalRequired,
		ShellAccessUnrestricted,
	} {
		if !access.Valid() {
			t.Fatalf("ShellAccess(%q).Valid() = false", access)
		}
		parsed, err := ParseShellAccess(string(access))
		if err != nil || parsed != access {
			t.Fatalf("ParseShellAccess(%q) = %q, %v", access, parsed, err)
		}
	}
	if ShellAccess("invalid").Valid() {
		t.Fatal("invalid ShellAccess.Valid() = true")
	}
	if _, err := ParseShellAccess("invalid"); err == nil {
		t.Fatal("ParseShellAccess(invalid) error = nil")
	}
}
