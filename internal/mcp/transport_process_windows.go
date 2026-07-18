package mcp

import (
	"os/exec"
	"syscall"
)

func configureStdioCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
