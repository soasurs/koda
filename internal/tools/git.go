package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/soasurs/adk/tool"
	"github.com/soasurs/koda/internal/permission"
)

type gitInput struct {
	Workdir    string   `json:"workdir,omitempty" jsonschema:"Git repository directory; defaults to the session workspace"`
	Subcommand string   `json:"subcommand" jsonschema:"Read-only Git subcommand: status, diff, log, show, blame, branch, rev-parse, or ls-files"`
	Args       []string `json:"args,omitempty" jsonschema:"Arguments for the allowed read-only subcommand"`
	MaxChars   int      `json:"max_chars,omitempty" jsonschema:"Maximum returned characters; defaults to 32768 and is capped"`
}

type gitOutput struct {
	Output    string `json:"output"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated"`
}

var readOnlyGitSubcommands = map[string]struct{}{
	"status":    {},
	"diff":      {},
	"log":       {},
	"show":      {},
	"blame":     {},
	"branch":    {},
	"rev-parse": {},
	"ls-files":  {},
}

func (s service) newGitTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "git",
		Description: "Run an allowlisted read-only Git subcommand without a shell. Mutating Git operations must use run_shell and require its approval policy.",
	}, s.git)
}

func (s service) git(ctx context.Context, input gitInput) (gitOutput, error) {
	if strings.TrimSpace(input.Workdir) == "" {
		input.Workdir = "."
	}
	workdir, err := s.resolver.existing(input.Workdir)
	if err != nil {
		return gitOutput{}, handled(err)
	}
	if !workdir.info.IsDir() {
		return gitOutput{}, handled(errors.New("workdir is not a directory"))
	}
	subcommand, err := validateGit(input.Subcommand, input.Args)
	if err != nil {
		return gitOutput{}, handled(err)
	}
	targets, scope := gitTargets(s.resolver, workdir)
	if err := s.authorize(ctx, permission.KindFileRead, scope, absoluteTargets(targets...), "git "+subcommand+" in "+workdir.display, nil); err != nil {
		return gitOutput{}, err
	}

	commandCtx, cancel := context.WithTimeout(ctx, s.commandTimeout)
	defer cancel()
	args := []string{"--no-pager", "-c", "core.pager=cat", subcommand}
	if subcommand == "diff" || subcommand == "log" || subcommand == "show" {
		args = append(args, "--no-ext-diff", "--no-textconv")
	}
	args = append(args, input.Args...)
	command := exec.CommandContext(commandCtx, "git", args...)
	command.Dir = workdir.real
	command.Env = append(os.Environ(), "GIT_PAGER=cat", "GIT_EXTERNAL_DIFF=", "GIT_TERMINAL_PROMPT=0")
	buffer := newTruncatingBuffer(clamp(input.MaxChars, defaultMaxChars, defaultMaxChars))
	command.Stdout = buffer
	command.Stderr = buffer
	err = command.Run()
	if ctx.Err() != nil {
		return gitOutput{}, ctx.Err()
	}
	if commandCtx.Err() != nil {
		return gitOutput{}, handled(errors.New("git command timed out"))
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return gitOutput{}, handled(fmt.Errorf("run git: %w", err))
		}
		exitCode = exitErr.ExitCode()
	}
	return gitOutput{Output: buffer.String(), ExitCode: exitCode, Truncated: buffer.truncated}, nil
}

func validateGit(subcommand string, args []string) (string, error) {
	subcommand = strings.TrimSpace(subcommand)
	if _, ok := readOnlyGitSubcommands[subcommand]; !ok {
		return "", fmt.Errorf("git subcommand %q is not allowed", subcommand)
	}
	for _, arg := range args {
		trimmed := strings.TrimSpace(arg)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "-c") || strings.HasPrefix(trimmed, "--config") ||
			trimmed == "--no-index" || strings.HasPrefix(trimmed, "--no-index=") ||
			trimmed == "--ext-diff" || strings.HasPrefix(trimmed, "--ext-diff=") ||
			trimmed == "--textconv" || strings.HasPrefix(trimmed, "--textconv=") ||
			trimmed == "--contents" || strings.HasPrefix(trimmed, "--contents=") ||
			trimmed == "--show-signature" ||
			trimmed == "--output" || strings.HasPrefix(trimmed, "--output=") ||
			trimmed == "--paginate" || strings.HasPrefix(trimmed, "--pager") {
			return "", fmt.Errorf("git argument %q is not allowed", arg)
		}
	}
	return subcommand, nil
}

func gitTargets(resolver resolver, workdir resolvedPath) ([]resolvedPath, permission.Scope) {
	targets := []resolvedPath{workdir}
	gitPath := filepath.Join(workdir.real, ".git")
	git, err := resolver.existing(gitPath)
	if err != nil {
		return targets, widestScope(targets...)
	}
	targets = append(targets, git)
	if git.info.IsDir() {
		if common, err := gitCommonDir(resolver, git.real); err == nil {
			targets = append(targets, common)
		}
		return targets, widestScope(targets...)
	}

	contents, err := os.ReadFile(git.real)
	if err != nil {
		return targets, widestScope(targets...)
	}
	const prefix = "gitdir:"
	value := strings.TrimSpace(string(contents))
	if !strings.HasPrefix(strings.ToLower(value), prefix) {
		return targets, widestScope(targets...)
	}
	gitdir := strings.TrimSpace(value[len(prefix):])
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(workdir.real, gitdir)
	}
	if directory, err := resolver.existing(gitdir); err == nil {
		targets = append(targets, directory)
		if common, err := gitCommonDir(resolver, directory.real); err == nil {
			targets = append(targets, common)
		}
	}
	return targets, widestScope(targets...)
}

func gitCommonDir(resolver resolver, gitDir string) (resolvedPath, error) {
	contents, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return resolvedPath{}, err
	}
	value := strings.TrimSpace(string(contents))
	if value == "" {
		return resolvedPath{}, errors.New("empty git common directory")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(gitDir, value)
	}
	return resolver.existing(value)
}

type truncatingBuffer struct {
	buffer    bytes.Buffer
	maximum   int
	truncated bool
}

func newTruncatingBuffer(maximum int) *truncatingBuffer {
	return &truncatingBuffer{maximum: maximum}
}

func (b *truncatingBuffer) Write(value []byte) (int, error) {
	remaining := b.maximum - b.buffer.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(value), nil
	}
	if len(value) > remaining {
		_, _ = b.buffer.Write(value[:remaining])
		b.truncated = true
		return len(value), nil
	}
	return b.buffer.Write(value)
}

func (b *truncatingBuffer) String() string {
	if !b.truncated {
		return b.buffer.String()
	}
	return b.buffer.String() + "\n… output truncated"
}

var _ io.Writer = (*truncatingBuffer)(nil)
