package agent

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	buildInstruction = `You are Koda, a coding agent working in the configured workspace. Inspect the codebase before changing it. Follow the workspace instructions below. Use tools deliberately, respect approval boundaries, and verify changes with the repository's relevant checks. Explain concise results to the user.`
	planInstruction  = `You are Koda in planning mode. Inspect and analyze the configured workspace, then help the user make an implementation plan. Do not claim to have changed files: planning mode has only read-only tools. Ask focused questions when a user decision materially affects the plan. Follow the workspace instructions below.`
)

// LoadWorkspaceInstructions returns the AGENTS.md files from the filesystem
// root through workdir, ordered so a closer file appears later and can refine
// instructions from its parents.
func LoadWorkspaceInstructions(workdir string) (string, error) {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return "", errors.New("agent: workdir must not be empty")
	}
	abs, err := filepath.Abs(workdir)
	if err != nil {
		return "", fmt.Errorf("agent: resolve workdir: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("agent: resolve workdir symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("agent: stat workdir: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("agent: workdir must be a directory")
	}

	var directories []string
	for directory := filepath.Clean(resolved); ; directory = filepath.Dir(directory) {
		directories = append(directories, directory)
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
	}

	var builder strings.Builder
	for index := len(directories) - 1; index >= 0; index-- {
		path := filepath.Join(directories[index], "AGENTS.md")
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("agent: read %s: %w", path, err)
		}
		contents := strings.TrimSpace(string(data))
		if contents == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		fmt.Fprintf(&builder, "Instructions from %s:\n%s", path, contents)
	}
	return builder.String(), nil
}

func instructionFor(mode Mode, workdir string) (string, [sha256.Size]byte, error) {
	workspaceInstructions, err := LoadWorkspaceInstructions(workdir)
	if err != nil {
		return "", [sha256.Size]byte{}, err
	}
	var instruction string
	switch mode {
	case ModeBuild:
		instruction = buildInstruction
	case ModePlan:
		instruction = planInstruction
	default:
		return "", [sha256.Size]byte{}, fmt.Errorf("agent: invalid mode %q", mode)
	}
	if workspaceInstructions != "" {
		instruction += "\n\n" + workspaceInstructions
	}
	return instruction, sha256.Sum256([]byte(instruction)), nil
}
