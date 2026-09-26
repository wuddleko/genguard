//go:build windows

package check

import "os/exec"

func setCommandGroup(cmd *exec.Cmd) {}

func stopCommand(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
