package check

import (
	"io"
	"os/exec"
	"strings"
	"time"
)

func RunCommand(root, command string) (string, error) {
	return runCommand(root, command, nil, "", 0)
}

func runCommand(root, command string, log io.Writer, header string, timeout time.Duration) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", newGenguardError("command is empty")
	}

	name, args := shellInvocation(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	var ring tailRing
	stream := newCommandStream(log, header)
	defer stream.finish()
	if stream.active {
		ring.onLine = stream.onLine
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
		msg := newGenguardError("command timed out after %s", timeout)
		if stream.streamed() {
			return "", msg
		}
		return ring.String(), msg
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
		return "", newGenguardError("command failed (exit %d): no output", exitCode)
	}
	if stream.streamed() {
		return "", newGenguardError("command failed (exit %d)", exitCode)
	}
	return tail, newGenguardError("command failed (exit %d)", exitCode)
}
