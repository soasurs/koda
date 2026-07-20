package agent

import (
	"context"
	"crypto/sha256"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/soasurs/adk/agent/llmagent"

	"github.com/soasurs/koda/internal/permission"
)

//go:embed prompts/*.md
var promptFiles embed.FS

func staticInstruction(mode Mode) (string, error) {
	if !mode.valid() {
		return "", fmt.Errorf("agent: invalid mode %q", mode)
	}
	common, err := embeddedPrompt("prompts/common.md")
	if err != nil {
		return "", err
	}
	modePrompt, err := embeddedPrompt("prompts/" + string(mode) + ".md")
	if err != nil {
		return "", err
	}
	return common + "\n\n" + modePrompt, nil
}

func embeddedPrompt(name string) (string, error) {
	data, err := promptFiles.ReadFile(name)
	if err != nil {
		return "", fmt.Errorf("agent: read embedded prompt %q: %w", name, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("agent: embedded prompt %q must not be empty", name)
	}
	return value, nil
}

func renderEmbeddedPrompt(name string, data any) (string, error) {
	source, err := embeddedPrompt(name)
	if err != nil {
		return "", err
	}
	prompt, err := template.New(name).Option("missingkey=error").Parse(source)
	if err != nil {
		return "", fmt.Errorf("agent: parse embedded prompt %q: %w", name, err)
	}
	var builder strings.Builder
	if err := prompt.Execute(&builder, data); err != nil {
		return "", fmt.Errorf("agent: render embedded prompt %q: %w", name, err)
	}
	return builder.String(), nil
}

// LoadWorkspaceInstructions reads the global ~/.koda/AGENTS.md and the
// workspace root AGENTS.md file. Subdirectory AGENTS.md files are loaded
// on demand with the load_instructions tool.
func LoadWorkspaceInstructions(workdir string) (string, error) {
	resolved, err := normalizeWorkdir(workdir)
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return loadWorkspaceInstructions(resolved, home)
}

func loadWorkspaceInstructions(workdir, homeDir string) (string, error) {
	var builder strings.Builder
	if homeDir != "" {
		if err := appendInstructionFile(&builder, filepath.Join(homeDir, ".koda", "AGENTS.md")); err != nil {
			return "", err
		}
	}
	if err := appendInstructionFile(&builder, filepath.Join(workdir, "AGENTS.md")); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func appendInstructionFile(builder *strings.Builder, path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("agent: read %s: %w", path, err)
	}
	contents := strings.TrimSpace(string(data))
	if contents == "" {
		return nil
	}
	if builder.Len() > 0 {
		builder.WriteString("\n\n")
	}
	fmt.Fprintf(builder, "## Instructions from %s\n\n%s", strconv.Quote(path), contents)
	return nil
}

func normalizeWorkdir(workdir string) (string, error) {
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
	return filepath.Clean(resolved), nil
}

func instructionConfiguration(mode Mode, workdir, skillInstruction string) (string, llmagent.InstructionProvider, [sha256.Size]byte, error) {
	static, err := staticInstruction(mode)
	if err != nil {
		return "", nil, [sha256.Size]byte{}, err
	}
	workspace, err := LoadWorkspaceInstructions(workdir)
	if err != nil {
		return "", nil, [sha256.Size]byte{}, err
	}
	hash := sha256.Sum256([]byte(static + "\x00" + workspace + "\x00" + skillInstruction))
	provider := func(ctx context.Context, _ llmagent.InstructionInput) (string, error) {
		environment, ok := RunEnvironmentFromContext(ctx)
		if !ok {
			return "", errors.New("agent: run environment is missing")
		}
		runtime, err := runtimeInstruction(mode, environment)
		if err != nil {
			return "", err
		}
		if workspace != "" {
			runtime += "\n\n# Workspace instructions\n\n" + workspace
		}
		runtime += "\n\nSubdirectory AGENTS.md files are loaded on demand with the `load_instructions` tool."
		if skillInstruction != "" {
			runtime += "\n\n# Available skills\n\n" + skillInstruction
		}
		return runtime, nil
	}
	return static, provider, hash, nil
}

func runtimeInstruction(mode Mode, environment RunEnvironment) (string, error) {
	workdir, err := normalizeWorkdir(environment.Workdir)
	if err != nil {
		return "", err
	}
	if !environment.FileAccess.Valid() {
		return "", fmt.Errorf("agent: invalid run environment file access %q", environment.FileAccess)
	}
	if !environment.ShellAccess.Valid() {
		return "", fmt.Errorf("agent: invalid run environment shell access %q", environment.ShellAccess)
	}
	if !mode.valid() {
		return "", fmt.Errorf("agent: invalid mode %q", mode)
	}

	var permissions string
	if mode == ModePlan {
		permissions = "Session permissions do not add tools in Plan mode. Filesystem tools are read-only. Shell access is limited to the tool's allowlisted read-only Git command, even when session shell access is unrestricted. Other commands, filesystem writes, and mutating Git operations are unavailable."
	} else {
		permissions = buildPermissions(environment.FileAccess, environment.ShellAccess)
	}
	return renderEmbeddedPrompt("prompts/runtime.md", struct {
		Workdir     string
		Mode        Mode
		FileAccess  permission.FileAccess
		ShellAccess permission.ShellAccess
		Permissions string
	}{
		Workdir:     strconv.Quote(workdir),
		Mode:        mode,
		FileAccess:  environment.FileAccess,
		ShellAccess: environment.ShellAccess,
		Permissions: permissions,
	})
}

func buildPermissions(fileAccess permission.FileAccess, shellAccess permission.ShellAccess) string {
	var files string
	switch fileAccess {
	case permission.FileAccessWorkspaceRead:
		files = "Workspace reads are automatic. Workspace writes and all access outside the workspace require approval."
	case permission.FileAccessWorkspaceWrite:
		files = "Workspace reads and writes are automatic. All access outside the workspace requires approval."
	case permission.FileAccessUnrestricted:
		files = "All filesystem reads and writes are automatic."
	}
	if shellAccess == permission.ShellAccessUnrestricted {
		return files + " Shell commands are automatic and have effective access to the full filesystem."
	}
	return files + " Every shell command requires approval."
}
