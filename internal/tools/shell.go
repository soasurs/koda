package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/soasurs/adk/tool"

	"github.com/soasurs/koda/internal/permission"
)

const maxShellTimeout = 5 * time.Minute

type runShellInput struct {
	Command        string `json:"command" jsonschema:"Shell command to execute"`
	Workdir        string `json:"workdir,omitempty" jsonschema:"Working directory; defaults to the session workspace"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"Maximum runtime in seconds; defaults to 30 and is capped at 300"`
	MaxChars       int    `json:"max_chars,omitempty" jsonschema:"Maximum returned characters; defaults to 32768 and is capped"`
}

type runShellOutput struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated"`
}

func (s service) newRunShellTool() (tool.Tool, error) {
	return tool.NewFunc(tool.Definition{
		Name:        "run_shell",
		Description: "Execute an arbitrary shell command. It requires approval by default because its filesystem and process effects cannot be predicted.",
	}, s.runShell)
}

func (s service) runShell(ctx context.Context, input runShellInput) (runShellOutput, error) {
	if strings.TrimSpace(input.Command) == "" {
		return runShellOutput{}, handled(errors.New("command must not be empty"))
	}
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
	if err := s.authorize(ctx, permission.KindShell, permission.ScopeGlobal, absoluteTargets(workdir), "run shell command in "+workdir.display, nil); err != nil {
		return runShellOutput{}, err
	}

	timeout := shellTimeout(s.commandTimeout, input.TimeoutSeconds)
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(commandCtx, "sh", "-c", input.Command)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
	command.WaitDelay = time.Second
	command.Dir = workdir.real
	stdout := newTruncatingBuffer(clamp(input.MaxChars, defaultMaxChars, defaultMaxChars))
	stderr := newTruncatingBuffer(clamp(input.MaxChars, defaultMaxChars, defaultMaxChars))
	command.Stdout = stdout
	command.Stderr = stderr
	err = command.Run()
	if ctx.Err() != nil {
		return runShellOutput{}, ctx.Err()
	}
	if commandCtx.Err() != nil {
		return runShellOutput{}, handled(fmt.Errorf("shell command timed out after %s", timeout))
	}
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return runShellOutput{}, handled(fmt.Errorf("run shell command: %w", err))
		}
		exitCode = exitErr.ExitCode()
	}
	return runShellOutput{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode,
		Truncated: stdout.truncated || stderr.truncated,
	}, nil
}

func shellTimeout(fallback time.Duration, seconds int) time.Duration {
	if seconds > 0 {
		return min(time.Duration(seconds)*time.Second, maxShellTimeout)
	}
	return fallback
}
