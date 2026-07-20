package tools

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/permission"
)

type loadInstructionsInput struct {
	Path string `json:"path,omitempty" jsonschema:"Subdirectory path, relative to the workspace unless absolute"`
}

type loadInstructionsOutput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (s service) newLoadInstructionsTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name: "load_instructions",
		Description: "Load AGENTS.md instructions from a workspace subdirectory. " +
			"Use this when entering a subdirectory to load directory-specific coding rules. " +
			"The workspace root AGENTS.md and global ~/.koda/AGENTS.md are already loaded automatically.",
	}, s.loadInstructions)
}

func (s service) loadInstructions(ctx context.Context, input loadInstructionsInput) (loadInstructionsOutput, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return loadInstructionsOutput{}, handled(errors.New("path must not be empty"))
	}

	resolved, err := s.resolver.existing(path)
	if err != nil {
		return loadInstructionsOutput{}, handled(fmt.Errorf("resolve path: %w", err))
	}
	if !resolved.info.IsDir() {
		return loadInstructionsOutput{}, handled(fmt.Errorf("%s is not a directory", strconv.Quote(resolved.display)))
	}
	if resolved.scope != permission.ScopeWorkspace {
		return loadInstructionsOutput{}, handled(fmt.Errorf("%s is outside the workspace", strconv.Quote(resolved.display)))
	}
	if resolved.real == resolved.workspace {
		return loadInstructionsOutput{}, handled(fmt.Errorf(
			"%s is the workspace root; its instructions are already loaded automatically",
			strconv.Quote(resolved.display),
		))
	}

	agentsPath := filepath.Join(resolved.real, "AGENTS.md")
	data, err := os.ReadFile(agentsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return loadInstructionsOutput{}, handled(fmt.Errorf(
			"no AGENTS.md found in %s", strconv.Quote(resolved.display),
		))
	}
	if err != nil {
		return loadInstructionsOutput{}, handled(fmt.Errorf("read %s: %w", agentsPath, err))
	}

	contents := strings.TrimSpace(string(data))
	if contents == "" {
		return loadInstructionsOutput{}, handled(fmt.Errorf(
			"AGENTS.md in %s is empty", strconv.Quote(resolved.display),
		))
	}

	return loadInstructionsOutput{
		Path:    resolved.display,
		Content: fmt.Sprintf("## Instructions from %s\n\n%s", strconv.Quote(resolved.display), contents),
	}, nil
}
