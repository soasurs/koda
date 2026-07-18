//go:build !windows

package tools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func newShellCommand(ctx context.Context, input string) *exec.Cmd {
	command := exec.CommandContext(ctx, "sh", "-c", input)
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
	return command
}
