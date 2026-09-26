//go:build unix

package check

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

func setCommandGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopCommand(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
}
