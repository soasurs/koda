package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func newShellCommand(ctx context.Context, input string) *exec.Cmd {
	command := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", input)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		output, err := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(command.Process.Pid)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("terminate process tree: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
	command.WaitDelay = time.Second
	return command
}
