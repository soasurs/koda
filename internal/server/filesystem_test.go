package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/soasurs/koda/gen/koda/v1"
)

func TestListDirectoriesRejectsNilRequest(t *testing.T) {
	response, err := (&Handler{}).ListDirectories(t.Context(), nil)
	if response != nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListDirectories() = %+v, %v, want nil, invalid_argument", response, err)
	}
}

func TestListDirectoriesUsesHomeForEmptyPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(home) error = %v", err)
	}
	response, err := (&Handler{}).ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{}.Build())
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	if response.GetPath() != filepath.Clean(want) {
		t.Fatalf("path = %q, want %q", response.GetPath(), filepath.Clean(want))
	}
}

func TestListDirectoriesRejectsRelativePath(t *testing.T) {
	response, err := (&Handler{}).ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{
		Path: new("relative/path"),
	}.Build())
	if response != nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListDirectories() = %+v, %v, want nil, invalid_argument", response, err)
	}
}

func TestListDirectoriesMapsMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	response, err := (&Handler{}).ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{
		Path: new(missing),
	}.Build())
	if response != nil || connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("ListDirectories() = %+v, %v, want nil, not_found", response, err)
	}
}

func TestListDirectoriesRejectsFilePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	response, err := (&Handler{}).ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{
		Path: new(path),
	}.Build())
	if response != nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ListDirectories() = %+v, %v, want nil, invalid_argument", response, err)
	}
}

func TestListDirectoriesReturnsOnlySortedImmediateDirectories(t *testing.T) {
	root := t.TempDir()
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks() error = %v", err)
	}
	for _, name := range []string{"zeta", "Alpha", "beta"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
			t.Fatalf("os.Mkdir(%q) error = %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "Alpha", "nested"), 0o700); err != nil {
		t.Fatalf("os.Mkdir(nested) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	response, err := (&Handler{}).ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{
		Path: new(root),
	}.Build())
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	gotNames := make([]string, 0, len(response.GetDirectories()))
	for _, directory := range response.GetDirectories() {
		gotNames = append(gotNames, directory.GetName())
		if directory.GetPath() != filepath.Join(canonicalRoot, directory.GetName()) {
			t.Errorf("directory %q path = %q", directory.GetName(), directory.GetPath())
		}
	}
	wantNames := []string{"Alpha", "beta", "zeta"}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("directory names = %q, want %q", gotNames, wantNames)
	}
	if response.GetPath() != canonicalRoot {
		t.Errorf("path = %q, want %q", response.GetPath(), canonicalRoot)
	}
	if response.GetParentPath() != filepath.Dir(canonicalRoot) {
		t.Errorf("parent path = %q, want %q", response.GetParentPath(), filepath.Dir(canonicalRoot))
	}
}

func TestListDirectoriesRootHasNoParent(t *testing.T) {
	temp := t.TempDir()
	root := filepath.VolumeName(temp) + string(filepath.Separator)
	response, err := (&Handler{}).ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{
		Path: new(root),
	}.Build())
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	if response.GetPath() != filepath.Clean(root) || response.GetParentPath() != "" {
		t.Fatalf("path = %q, parent path = %q, want %q, empty", response.GetPath(), response.GetParentPath(), filepath.Clean(root))
	}
}

func TestListDirectoriesCanonicalizesCurrentSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	link := filepath.Join(root, "link")
	createTestSymlink(t, target, link)
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks() error = %v", err)
	}

	response, err := (&Handler{}).ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{
		Path: new(link),
	}.Build())
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	if response.GetPath() != canonicalTarget || response.GetParentPath() != filepath.Dir(canonicalTarget) {
		t.Fatalf("path = %q, parent path = %q, want %q, %q", response.GetPath(), response.GetParentPath(), canonicalTarget, filepath.Dir(canonicalTarget))
	}
}

func TestListDirectoriesFiltersSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	targetDirectory := filepath.Join(root, "target-directory")
	if err := os.Mkdir(targetDirectory, 0o700); err != nil {
		t.Fatalf("os.Mkdir() error = %v", err)
	}
	targetFile := filepath.Join(root, "target-file")
	if err := os.WriteFile(targetFile, []byte("content"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	createTestSymlink(t, targetDirectory, filepath.Join(root, "directory-link"))
	createTestSymlink(t, targetFile, filepath.Join(root, "file-link"))
	createTestSymlink(t, filepath.Join(root, "missing"), filepath.Join(root, "broken-link"))

	response, err := (&Handler{}).ListDirectories(t.Context(), v1.ListDirectoriesRequest_builder{
		Path: new(root),
	}.Build())
	if err != nil {
		t.Fatalf("ListDirectories() error = %v", err)
	}
	gotNames := make([]string, 0, len(response.GetDirectories()))
	for _, directory := range response.GetDirectories() {
		gotNames = append(gotNames, directory.GetName())
	}
	wantNames := []string{"directory-link", "target-directory"}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("directory names = %q, want %q", gotNames, wantNames)
	}
}

func TestFilesystemErrorMappings(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want connect.Code
	}{
		{name: "permission", err: os.ErrPermission, want: connect.CodePermissionDenied},
		{name: "canceled", err: context.Canceled, want: connect.CodeCanceled},
		{name: "deadline", err: context.DeadlineExceeded, want: connect.CodeDeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := connect.CodeOf(filesystemError(test.err)); got != test.want {
				t.Fatalf("filesystemError() code = %v, want %v", got, test.want)
			}
		})
	}
}

func TestListDirectoriesMapsCanceledContexts(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	deadline, deadlineCancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer deadlineCancel()
	for _, test := range []struct {
		name string
		ctx  context.Context
		want connect.Code
	}{
		{name: "canceled", ctx: canceled, want: connect.CodeCanceled},
		{name: "deadline", ctx: deadline, want: connect.CodeDeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := (&Handler{}).ListDirectories(test.ctx, v1.ListDirectoriesRequest_builder{
				Path: new(t.TempDir()),
			}.Build())
			if response != nil || connect.CodeOf(err) != test.want {
				t.Fatalf("ListDirectories() = %+v, %v, want nil, %v", response, err, test.want)
			}
		})
	}
}

func createTestSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is not supported: %v", err)
	}
}

func TestFilesystemErrorHidesUnknownDetails(t *testing.T) {
	err := filesystemError(errors.New("sensitive system detail"))
	if connect.CodeOf(err) != connect.CodeInternal || err.Error() != "internal: list directories failed" {
		t.Fatalf("filesystemError() = %v, want safe internal error", err)
	}
}
