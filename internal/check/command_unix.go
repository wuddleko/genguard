//go:build unix

package check

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

type commandGroup struct{}

func startCommand(cmd *exec.Cmd) (commandGroup, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return commandGroup{}, cmd.Start()
}

func (commandGroup) stop(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
}

func (commandGroup) release() {}
