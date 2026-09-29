//go:build darwin

package command

import "golang.org/x/sys/unix"

func killGroup(pid int) {
	pids := append(descendantPIDs(pid), pid)
	_ = unix.Kill(-pid, unix.SIGKILL)
	for _, id := range pids {
		_ = unix.Kill(id, unix.SIGKILL)
	}
}

func descendantPIDs(root int) []int {
	if root <= 0 {
		return nil
	}
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil
	}
	children := make(map[int][]int)
	for _, proc := range procs {
		pid := int(proc.Proc.P_pid)
		ppid := int(proc.Eproc.Ppid)
		if pid <= 0 || ppid <= 0 {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}
	seen := map[int]bool{root: true}
	var out []int
	queue := []int{root}
	for i := 0; i < len(queue); i++ {
		for _, child := range children[queue[i]] {
			if seen[child] {
				continue
			}
			seen[child] = true
			out = append(out, child)
			queue = append(queue, child)
		}
	}
	return out
}
