//go:build !unix && !windows

package command

import "os/exec"

type commandGroup struct{}

func startCommand(cmd *exec.Cmd) (commandGroup, error) {
	return commandGroup{}, cmd.Start()
}

func (commandGroup) stop(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}

func (commandGroup) release() {}
