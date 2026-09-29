//go:build unix

package command

import (
	"os/exec"
	"syscall"
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
	killGroup(cmd.Process.Pid)
}

func (commandGroup) release() {}

func setCmdLine(_ *exec.Cmd, _ string) {}
