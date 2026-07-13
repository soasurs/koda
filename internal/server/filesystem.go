package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	v1 "github.com/soasurs/koda/gen/koda/v1"
)

// ListDirectories returns the immediate child directories of one local path.
func (h *Handler) ListDirectories(ctx context.Context, request *v1.ListDirectoriesRequest) (*v1.ListDirectoriesResponse, error) {
	if request == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("list directories request must not be nil"))
	}
	if err := ctx.Err(); err != nil {
		return nil, filesystemError(err)
	}

	path := request.GetPath()
	if path == "" {
		var err error
		path, err = os.UserHomeDir()
		if err != nil {
			return nil, filesystemError(err)
		}
		if !filepath.IsAbs(path) {
			return nil, filesystemError(errors.New("user home directory is not absolute"))
		}
	} else if !filepath.IsAbs(path) {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("directory path must be absolute"))
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, filesystemError(err)
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, filesystemError(err)
	}
	if !info.IsDir() {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("directory path must identify a directory"))
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, filesystemError(err)
	}
	directories := make([]*v1.DirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, filesystemError(err)
		}
		entryPath := filepath.Join(resolved, entry.Name())
		isDirectory := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			entryInfo, err := os.Stat(entryPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return nil, filesystemError(err)
			}
			isDirectory = entryInfo.IsDir()
		}
		if !isDirectory {
			continue
		}
		directories = append(directories, v1.DirectoryEntry_builder{
			Name: proto.String(entry.Name()),
			Path: proto.String(entryPath),
		}.Build())
	}

	parentPath := filepath.Dir(resolved)
	if parentPath == resolved {
		parentPath = ""
	}
	return v1.ListDirectoriesResponse_builder{
		Path:        proto.String(resolved),
		ParentPath:  proto.String(parentPath),
		Directories: directories,
	}.Build(), nil
}

func filesystemError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case errors.Is(err, os.ErrNotExist):
		return connect.NewError(connect.CodeNotFound, errors.New("directory not found"))
	case errors.Is(err, os.ErrPermission):
		return connect.NewError(connect.CodePermissionDenied, errors.New("directory access denied"))
	default:
		return connect.NewError(connect.CodeInternal, errors.New("list directories failed"))
	}
}
