package tools

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/permission"
)

var planGitSubcommands = map[string]struct{}{
	"status":    {},
	"diff":      {},
	"log":       {},
	"show":      {},
	"blame":     {},
	"branch":    {},
	"rev-parse": {},
	"ls-files":  {},
}

var errUnsafePlanGitMetadata = errors.New("unsafe Plan mode Git metadata")

var planGitBranchArgs = map[string]struct{}{
	"--list":         {},
	"--show-current": {},
	"--all":          {},
	"--remotes":      {},
	"--verbose":      {},
	"--no-color":     {},
	"--color=never":  {},
	"-a":             {},
	"-r":             {},
	"-v":             {},
	"-vv":            {},
}

func (s service) newPlanShellTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "run_shell",
		Description: "Run one read-only Git command in Plan mode. Other commands and mutating Git operations are rejected.",
	}, s.runPlanShell)
}

func (s service) runPlanShell(ctx context.Context, input runShellInput) (runShellOutput, error) {
	if strings.TrimSpace(input.Workdir) == "" {
		input.Workdir = "."
	}
	workdir, err := s.resolver.existing(input.Workdir)
	if err != nil {
		return runShellOutput{}, handled(err)
	}
	if !workdir.info.IsDir() {
		return runShellOutput{}, handled(errors.New("workdir is not a directory"))
	}
	subcommand, args, err := parsePlanGitCommand(input.Command)
	if err != nil {
		return runShellOutput{}, handled(err)
	}
	targets, repositoryRoot, gitMetadata, repositoryFound := planGitBaseTargets(s.resolver, workdir)
	scope := widestScope(targets...)
	if err := s.authorize(ctx, permission.KindFileRead, scope, absoluteTargets(targets...), "git "+subcommand+" in "+workdir.display, nil); err != nil {
		return runShellOutput{}, err
	}
	var configPaths []string
	if repositoryFound {
		expanded, configs, expandErr := expandPlanGitTargets(s.resolver, repositoryRoot, gitMetadata)
		if expandErr != nil {
			return runShellOutput{}, handled(expandErr)
		}
		if len(expanded) > len(targets) {
			targets = expanded
			scope = widestScope(targets...)
			if err := s.authorize(ctx, permission.KindFileRead, scope, absoluteTargets(targets...), "git "+subcommand+" in "+workdir.display, nil); err != nil {
				return runShellOutput{}, err
			}
		}
		configPaths = configs
	}

	timeout := shellTimeout(s.commandTimeout, input.TimeoutSeconds)
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	safetyArgs, err := planGitSafetyArgs(commandCtx, configPaths)
	if err != nil {
		return runShellOutput{}, handled(err)
	}
	commandArgs := []string{
		"--no-pager", "--no-optional-locks",
		"-c", "core.pager=cat",
		"-c", "core.fsmonitor=false",
		"-c", "core.attributesFile=/dev/null",
		"-c", "core.excludesFile=/dev/null",
		"-c", "diff.submodule=short",
		"-c", "log.showSignature=false",
	}
	commandArgs = append(commandArgs, safetyArgs...)
	commandArgs = append(commandArgs, subcommand)
	if subcommand == "status" || subcommand == "diff" {
		commandArgs = append(commandArgs, "--ignore-submodules=all")
	}
	if subcommand == "diff" || subcommand == "log" || subcommand == "show" {
		commandArgs = append(commandArgs, "--no-ext-diff", "--no-textconv")
	}
	commandArgs = append(commandArgs, args...)
	command := exec.CommandContext(commandCtx, "git", commandArgs...)
	command.Dir = workdir.real
	command.Env = planGitEnvironment()
	buffer := newTruncatingBuffer(clamp(input.MaxChars, defaultMaxChars, defaultMaxChars))
	command.Stdout = buffer
	command.Stderr = buffer
	err = command.Run()
	if ctx.Err() != nil {
		return runShellOutput{}, ctx.Err()
	}
	if commandCtx.Err() != nil {
		s.logger.WarnContext(ctx, "Plan mode Git command timed out", "timeout", timeout, "subcommand", subcommand)
		return runShellOutput{}, handled(fmt.Errorf("git command timed out after %s", timeout))
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return runShellOutput{}, handled(fmt.Errorf("run git: %w", err))
		}
		exitCode = exitErr.ExitCode()
	}
	return runShellOutput{
		Stdout:    buffer.String(),
		ExitCode:  exitCode,
		Truncated: buffer.truncated,
	}, nil
}

func parsePlanGitCommand(command string) (string, []string, error) {
	fields, err := splitPlanCommand(command)
	if err != nil {
		return "", nil, err
	}
	if len(fields) < 2 || fields[0] != "git" {
		return "", nil, errors.New("Plan mode run_shell accepts only a read-only Git command")
	}
	subcommand := fields[1]
	if _, ok := planGitSubcommands[subcommand]; !ok {
		return "", nil, fmt.Errorf("git subcommand %q is not allowed in Plan mode", subcommand)
	}
	args := fields[2:]
	if subcommand == "branch" {
		for _, arg := range args {
			if _, ok := planGitBranchArgs[arg]; !ok {
				return "", nil, fmt.Errorf("git branch argument %q is not allowed in Plan mode", arg)
			}
		}
		if len(args) == 1 && args[0] == "--show-current" {
			return subcommand, args, nil
		}
		if len(args) == 0 || args[0] != "--list" {
			args = append([]string{"--list"}, args...)
		}
		return subcommand, args, nil
	}
	for _, arg := range args {
		if forbiddenPlanGitArgument(arg) {
			return "", nil, fmt.Errorf("git argument %q is not allowed in Plan mode", arg)
		}
	}
	return subcommand, args, nil
}

func splitPlanCommand(command string) ([]string, error) {
	var fields []string
	var current strings.Builder
	var quote rune
	escaped := false
	hasValue := false
	flush := func() {
		if hasValue {
			fields = append(fields, current.String())
			current.Reset()
			hasValue = false
		}
	}
	for _, value := range command {
		if escaped {
			current.WriteRune(value)
			hasValue = true
			escaped = false
			continue
		}
		if value == '\\' && quote != '\'' {
			escaped = true
			hasValue = true
			continue
		}
		if quote != 0 {
			if value == quote {
				quote = 0
			} else {
				current.WriteRune(value)
			}
			hasValue = true
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			hasValue = true
			continue
		}
		if unicode.IsSpace(value) {
			flush()
			continue
		}
		current.WriteRune(value)
		hasValue = true
	}
	if escaped || quote != 0 {
		return nil, errors.New("invalid quoted command")
	}
	flush()
	return fields, nil
}

func forbiddenPlanGitArgument(arg string) bool {
	return strings.HasPrefix(arg, "-c") || strings.HasPrefix(arg, "--config") ||
		arg == "--no-index" || strings.HasPrefix(arg, "--no-index=") ||
		arg == "--ext-diff" || strings.HasPrefix(arg, "--ext-diff=") ||
		arg == "--textconv" || strings.HasPrefix(arg, "--textconv=") ||
		arg == "--contents" || strings.HasPrefix(arg, "--contents=") ||
		arg == "--ignore-revs-file" || strings.HasPrefix(arg, "--ignore-revs-file=") ||
		arg == "--ignore-submodules" || strings.HasPrefix(arg, "--ignore-submodules=") ||
		arg == "--submodule" || strings.HasPrefix(arg, "--submodule=") ||
		arg == "--recurse-submodules" || strings.HasPrefix(arg, "--recurse-submodules=") ||
		arg == "--show-signature" ||
		strings.Contains(arg, "%G") ||
		arg == "--help" ||
		arg == "--output" || strings.HasPrefix(arg, "--output=") ||
		arg == "--paginate" || strings.HasPrefix(arg, "--pager")
}

func planGitBaseTargets(resolver resolver, workdir resolvedPath) ([]resolvedPath, resolvedPath, resolvedPath, bool) {
	targets := []resolvedPath{workdir}
	repositoryRoot, git, ok := findPlanGitRoot(resolver, workdir)
	if !ok {
		return targets, resolvedPath{}, resolvedPath{}, false
	}
	if repositoryRoot.real != workdir.real {
		targets = append(targets, repositoryRoot)
	}
	targets = append(targets, git)
	return targets, repositoryRoot, git, true
}

func expandPlanGitTargets(resolver resolver, repositoryRoot, git resolvedPath) ([]resolvedPath, []string, error) {
	targets := []resolvedPath{repositoryRoot, git}
	gitDirectory := git
	if git.info.IsDir() {
		if common, err := planGitCommonDir(resolver, git.real); err == nil {
			targets = append(targets, common)
			extraTargets, configs := planGitConfigPaths(resolver, git, common)
			return append(targets, extraTargets...), configs, nil
		} else if errors.Is(err, errUnsafePlanGitMetadata) {
			return nil, nil, err
		}
		extraTargets, configs := planGitConfigPaths(resolver, git, git)
		return append(targets, extraTargets...), configs, nil
	}

	contents, err := os.ReadFile(git.real)
	if err != nil {
		return targets, nil, nil
	}
	const prefix = "gitdir:"
	value := strings.TrimSpace(string(contents))
	if !strings.HasPrefix(strings.ToLower(value), prefix) {
		return targets, nil, nil
	}
	gitDir := strings.TrimSpace(value[len(prefix):])
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repositoryRoot.real, gitDir)
	}
	if directory, err := resolver.existing(gitDir); err == nil {
		gitDirectory = directory
		targets = append(targets, directory)
		if common, err := planGitCommonDir(resolver, directory.real); err == nil {
			targets = append(targets, common)
			extraTargets, configs := planGitConfigPaths(resolver, gitDirectory, common)
			return append(targets, extraTargets...), configs, nil
		} else if errors.Is(err, errUnsafePlanGitMetadata) {
			return nil, nil, err
		}
	}
	extraTargets, configs := planGitConfigPaths(resolver, gitDirectory, gitDirectory)
	return append(targets, extraTargets...), configs, nil
}

func planGitConfigPaths(resolver resolver, gitDirectory, commonDirectory resolvedPath) ([]resolvedPath, []string) {
	paths := []string{filepath.Join(commonDirectory.real, "config")}
	worktreeConfig := filepath.Join(gitDirectory.real, "config.worktree")
	if worktreeConfig != paths[0] {
		paths = append(paths, worktreeConfig)
	}
	var targets []resolvedPath
	configs := make([]string, 0, len(paths))
	for _, path := range paths {
		resolved, err := resolver.existing(path)
		if err != nil {
			continue
		}
		configs = append(configs, resolved.real)
		if !pathCoveredBy(resolved.real, gitDirectory.real) && !pathCoveredBy(resolved.real, commonDirectory.real) {
			targets = append(targets, resolved)
		}
	}
	return targets, configs
}

func pathCoveredBy(path, directory string) bool {
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func planGitSafetyArgs(ctx context.Context, configPaths []string) ([]string, error) {
	seen := make(map[string]struct{})
	var args []string
	for _, path := range configPaths {
		contents, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read Git configuration: %w", err)
		}
		scanner := bufio.NewScanner(strings.NewReader(string(contents)))
		for scanner.Scan() {
			section := strings.ToLower(strings.TrimSpace(scanner.Text()))
			if strings.HasPrefix(section, "[include") {
				return nil, errors.New("Plan mode Git rejects repository configuration includes")
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan Git configuration: %w", err)
		}
		command := exec.CommandContext(ctx, "git", "config", "--file", path, "--no-includes", "--null", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`)
		command.Env = planGitEnvironment()
		output, err := command.Output()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
				continue
			}
			return nil, fmt.Errorf("inspect Git filters: %w", err)
		}
		for encodedKey := range bytes.SplitSeq(output, []byte{0}) {
			key := string(encodedKey)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			value := ""
			if strings.HasSuffix(strings.ToLower(key), ".required") {
				value = "false"
			}
			args = append(args, "-c", key+"="+value)
		}
	}
	return args, nil
}

func findPlanGitRoot(resolver resolver, workdir resolvedPath) (resolvedPath, resolvedPath, bool) {
	for directory := workdir.real; ; directory = filepath.Dir(directory) {
		git, err := resolver.existing(filepath.Join(directory, ".git"))
		if err == nil {
			root, rootErr := resolver.existing(directory)
			if rootErr == nil {
				return root, git, true
			}
			return resolvedPath{}, resolvedPath{}, false
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return resolvedPath{}, resolvedPath{}, false
		}
	}
}

func planGitCommonDir(resolver resolver, gitDir string) (resolvedPath, error) {
	path := filepath.Join(gitDir, "commondir")
	info, err := os.Lstat(path)
	if err != nil {
		return resolvedPath{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return resolvedPath{}, fmt.Errorf("%w: symbolic-link common directory file", errUnsafePlanGitMetadata)
	}
	contents, err := os.ReadFile(path)
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

func cleanGitEnvironment() []string {
	result := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if forbiddenGitEnvironment(name) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func planGitEnvironment() []string {
	return append(cleanGitEnvironment(),
		"GIT_PAGER=cat",
		"GIT_EXTERNAL_DIFF=",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_ATTR_NOSYSTEM=1",
	)
}

func forbiddenGitEnvironment(name string) bool {
	return strings.HasPrefix(name, "GIT_")
}
