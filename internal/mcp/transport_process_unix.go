//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

func configureStdioCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
