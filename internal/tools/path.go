package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/soasurs/koda/internal/permission"
)

// resolver resolves a session-relative path and classifies its real location.
// It follows symlinks before assigning a workspace scope so an in-workspace
// symlink to an external target cannot bypass approval.
type resolver struct {
	workspace string
}

type resolvedPath struct {
	input     string
	abs       string
	real      string
	display   string
	scope     permission.Scope
	info      fs.FileInfo
	exists    bool
	workspace string
}

func (r resolver) existing(path string) (resolvedPath, error) {
	abs, err := r.absolute(path)
	if err != nil {
		return resolvedPath{}, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return resolvedPath{}, fmt.Errorf("resolve path %q: %w", path, err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return resolvedPath{}, fmt.Errorf("stat path %q: %w", path, err)
	}
	return r.resolved(path, abs, real, info, true), nil
}

// writeTarget resolves a possibly nonexistent path. Existing symlinks are
// followed; for a new path, the closest existing ancestor is resolved first.
func (r resolver) writeTarget(path string) (resolvedPath, error) {
	abs, err := r.absolute(path)
	if err != nil {
		return resolvedPath{}, err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		info, statErr := os.Stat(real)
		if statErr != nil {
			return resolvedPath{}, fmt.Errorf("stat path %q: %w", path, statErr)
		}
		return r.resolved(path, abs, real, info, true), nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return resolvedPath{}, fmt.Errorf("resolve path %q: %w", path, err)
	}

	real, err := resolveFuturePath(abs)
	if err != nil {
		return resolvedPath{}, fmt.Errorf("resolve path %q: %w", path, err)
	}
	return r.resolved(path, abs, real, nil, false), nil
}

func (r resolver) absolute(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path must not be empty")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return "", errors.New("path must not contain a NUL byte")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.workspace, path)
	}
	return filepath.Clean(path), nil
}

func (r resolver) resolved(input, abs, real string, info fs.FileInfo, exists bool) resolvedPath {
	real = filepath.Clean(real)
	scope := permission.ScopeOutsideWorkspace
	display := real
	if relative, err := filepath.Rel(r.workspace, real); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative) {
		scope = permission.ScopeWorkspace
		display = relative
		if display == "." {
			display = "."
		}
	}
	return resolvedPath{
		input:     input,
		abs:       abs,
		real:      real,
		display:   display,
		scope:     scope,
		info:      info,
		exists:    exists,
		workspace: r.workspace,
	}
}

func resolveFuturePath(path string) (string, error) {
	current := path
	var suffix []string
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				resolved, resolveErr := filepath.EvalSymlinks(current)
				if resolveErr != nil {
					return "", resolveErr
				}
				return filepath.Join(append([]string{resolved}, reverseStrings(suffix)...)...), nil
			}
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return "", resolveErr
			}
			return filepath.Join(append([]string{resolved}, reverseStrings(suffix)...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent, base := filepath.Dir(current), filepath.Base(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, base)
		current = parent
	}
}

func reverseStrings(values []string) []string {
	result := make([]string, len(values))
	for i := range values {
		result[len(values)-1-i] = values[i]
	}
	return result
}

func widestScope(paths ...resolvedPath) permission.Scope {
	for _, path := range paths {
		if path.scope != permission.ScopeWorkspace {
			return permission.ScopeOutsideWorkspace
		}
	}
	return permission.ScopeWorkspace
}

func absoluteTargets(paths ...resolvedPath) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		result = append(result, path.real)
	}
	return result
}
