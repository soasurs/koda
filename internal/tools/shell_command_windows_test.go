package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/soasurs/koda/internal/permission"
)

func shellPrintCommand(value string) string {
	return "[Console]::Out.Write('" + value + "')"
}

func shellFailureCommand() string {
	return "[Console]::Error.Write('error'); exit 3"
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
		Command:        "$child = Start-Process powershell.exe -ArgumentList '-NoProfile','-Command','Start-Sleep 30' -PassThru; $child.Id | Set-Content child.pid; Wait-Process $child.Id",
		TimeoutSeconds: 5,
	})
	encodedPID, err := os.ReadFile(filepath.Join(workspace, "child.pid"))
	if err != nil {
		t.Fatalf("ReadFile(child.pid) error = %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(encodedPID)))
	if err != nil {
		t.Fatalf("Atoi(child PID) error = %v", err)
	}
	t.Cleanup(func() {
		_ = exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
	})
	for deadline := time.Now().Add(time.Second); ; {
		err = exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Get-Process -Id "+strconv.Itoa(pid)+" -ErrorAction Stop | Out-Null").Run()
		if err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived shell timeout", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
