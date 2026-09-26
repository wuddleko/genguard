package command

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func Run(root, command string, onLine func(string), timeout time.Duration) (string, error) {
	return run(root, command, onLine, timeout)
}

func run(root, command string, onLine func(string), timeout time.Duration) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("command is empty")
	}

	name, args := shellInvocation(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	var ring tailRing
	if onLine != nil {
		ring.onLine = onLine
	}
	cmd.Stdout = &ring
	cmd.Stderr = &ring
	var err error
	var timedOut bool
	if timeout > 0 {
		var group commandGroup
		group, err = startCommand(cmd)
		if err == nil {
			defer group.release()
			wait := make(chan error, 1)
			go func() { wait <- cmd.Wait() }()
			timer := time.NewTimer(timeout)
			select {
			case err = <-wait:
				if !timer.Stop() {
					<-timer.C
				}
			case <-timer.C:
				timedOut = true
				group.stop(cmd)
				err = <-wait
			}
		}
	} else {
		err = cmd.Run()
	}
	ring.flush()
	if timedOut {
		return ring.String(), fmt.Errorf("command timed out after %s", timeout)
	}
	if err == nil {
		return "", nil
	}

	exitCode := 1
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	tail := ring.String()
	if tail == "" {
		return "", fmt.Errorf("command failed (exit %d): no output", exitCode)
	}
	return tail, fmt.Errorf("command failed (exit %d)", exitCode)
}

func errorsAsExit(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}
