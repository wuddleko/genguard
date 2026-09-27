//go:build unix && !darwin

package command

import "golang.org/x/sys/unix"

func killGroup(pid int) {
	_ = unix.Kill(-pid, unix.SIGKILL)
}
