//go:build !windows

package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/soasurs/koda/internal/permission"
)

func shellPrintCommand(value string) string {
	return "printf " + value
}

func shellFailureCommand() string {
	return "printf error >&2; exit 3"
}

func TestRunShellTimeoutKillsChildProcesses(t *testing.T) {
	workspace := t.TempDir()
	values, err := NewBuild(Config{
		Workdir: workspace, ShellAccess: permission.ShellAccessUnrestricted,
	})
	if err != nil {
		t.Fatalf("NewBuild() error = %v", err)
	}
	callToolError(t, toolByName(t, values, "run_shell"), runShellInput{
		Command: "sleep 30 & echo $! > child.pid; wait", TimeoutSeconds: 1,
	})
	encodedPID, err := os.ReadFile(filepath.Join(workspace, "child.pid"))
	if err != nil {
		t.Fatalf("ReadFile(child.pid) error = %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(encodedPID)))
	if err != nil {
		t.Fatalf("Atoi(child PID) error = %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	for deadline := time.Now().Add(time.Second); ; {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived shell timeout: %v", pid, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
