// Package skills loads Koda's process-level Agent Skills catalog.
package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	adkskill "github.com/soasurs/adk/skill"
)

// DefaultPath returns the default Agent Skills directory.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("skills: find home directory: %w", err)
	}
	return filepath.Join(home, ".koda", "skills"), nil
}

// LoadDefault loads Agent Skills from DefaultPath. Invalid individual skills
// are logged and skipped.
func LoadDefault(logger *slog.Logger) (*adkskill.Catalog, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return Load(path, logger)
}

// Load discovers Agent Skills in the direct child directories of path. A
// missing directory returns an empty catalog. Invalid individual skills are
// logged and skipped.
func Load(root string, logger *slog.Logger) (*adkskill.Catalog, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("skills: path must not be empty")
	}
	info, err := os.Stat(root)
	if errors.Is(err, os.ErrNotExist) {
		return adkskill.NewCatalog()
	}
	if err != nil {
		return nil, fmt.Errorf("skills: stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills: path %s must be a directory", root)
	}
	logger = loggerOrDiscard(logger)
	fsys := os.DirFS(root)
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("skills: read %s: %w", root, err)
	}
	loaded := make([]adkskill.Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		directory := entry.Name()
		skillFile := path.Join(directory, "SKILL.md")
		if _, err := fs.Stat(fsys, skillFile); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			logger.Error("load skill failed", "skill", directory, "path", filepath.Join(root, directory), "error", err)
			continue
		}
		skill, err := adkskill.Load(fsys, directory)
		if err != nil {
			logger.Error("load skill failed", "skill", directory, "path", filepath.Join(root, directory), "error", err)
			continue
		}
		loaded = append(loaded, skill)
	}
	catalog, err := adkskill.NewCatalog(loaded...)
	if err != nil {
		return nil, fmt.Errorf("skills: build catalog: %w", err)
	}
	return catalog, nil
}

func loggerOrDiscard(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return logger
}
