package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

var ErrInterrupted = errors.New("interrupted")

var waitDelay = 10 * time.Second

// Run runs command with sh -c in root. The tail of its output is returned
// only when it fails.
func Run(ctx context.Context, root, command string, onLine func(string), timeout time.Duration) (string, error) {
	text, code, err := execute(ctx, root, command, onLine, timeout)
	if err != nil || code != 0 {
		return text, err
	}
	return "", nil
}

// Capture runs command like Run and returns its output and exit code.
func Capture(ctx context.Context, root, command string, timeout time.Duration) (string, int, error) {
	return execute(ctx, root, command, nil, timeout)
}

func execute(ctx context.Context, root, command string, onLine func(string), timeout time.Duration) (string, int, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", 0, fmt.Errorf("command is empty")
	}
	if interrupted(ctx) {
		return "", 0, ErrInterrupted
	}

	name, args, cmdLine := shellInvocation(command)
	cmd := exec.Command(name, args...)
	setCmdLine(cmd, cmdLine)
	cmd.Dir = root
	var ring tailRing
	if onLine != nil {
		ring.onLine = onLine
	}
	cmd.Stdout = &ring
	cmd.Stderr = &ring
	started, err := waitCommand(ctx, cmd, timeout)
	ring.flush()
	if errors.Is(err, ErrInterrupted) {
		return ring.String(), 0, ErrInterrupted
	}
	var timedOut *TimeoutError
	if errors.As(err, &timedOut) {
		return ring.String(), 0, err
	}
	if err != nil && !started {
		return "", 0, &StartError{err: err}
	}
	if err == nil {
		return ring.String(), 0, nil
	}

	exitCode := 1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	tail := ring.String()
	if tail == "" {
		return "", exitCode, fmt.Errorf("command failed (exit %d): no output", exitCode)
	}
	return tail, exitCode, fmt.Errorf("command failed (exit %d)", exitCode)
}

func Output(ctx context.Context, name string, args ...string) (string, string, error) {
	if interrupted(ctx) {
		return "", "", ErrInterrupted
	}
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_, err := waitCommand(ctx, cmd, 0)
	if errors.Is(err, ErrInterrupted) {
		return stdout.String(), stderr.String(), ErrInterrupted
	}
	return stdout.String(), stderr.String(), err
}

func waitCommand(parent context.Context, cmd *exec.Cmd, timeout time.Duration) (bool, error) {
	cmd.WaitDelay = waitDelay
	if interrupted(parent) {
		return false, ErrInterrupted
	}

	group, err := startCommand(cmd)
	if cmd.Process == nil {
		return false, err
	}
	defer group.release()
	if err != nil {
		return true, err
	}

	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	timer, timerC := timeoutTimer(timeout)
	var done <-chan struct{}
	if watchable(parent) {
		done = parent.Done()
	}
	select {
	case err = <-wait:
		stopTimer(timer)
		if interrupted(parent) {
			group.stop(cmd)
			return true, ErrInterrupted
		}
		if errors.Is(err, exec.ErrWaitDelay) && succeeded(cmd) {
			group.stop(cmd)
			return true, nil
		}
		return true, err
	case <-done:
		stopTimer(timer)
		group.stop(cmd)
		<-wait
		return true, ErrInterrupted
	case <-timerC:
		group.stop(cmd)
		<-wait
		if interrupted(parent) {
			return true, ErrInterrupted
		}
		return true, &TimeoutError{Limit: timeout}
	}
}

func timeoutTimer(timeout time.Duration) (*time.Timer, <-chan time.Time) {
	if timeout <= 0 {
		return nil, nil
	}
	timer := time.NewTimer(timeout)
	return timer, timer.C
}

func stopTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func watchable(ctx context.Context) bool {
	return ctx != nil && ctx.Done() != nil
}

func interrupted(ctx context.Context) bool {
	return watchable(ctx) && ctx.Err() != nil
}

func succeeded(cmd *exec.Cmd) bool {
	return cmd.ProcessState != nil && cmd.ProcessState.Success()
}

type TimeoutError struct {
	Limit time.Duration
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("command timed out after %s", e.Limit)
}

type StartError struct {
	err error
}

func (e *StartError) Error() string {
	return "command failed to start: " + e.err.Error()
}

func (e *StartError) Unwrap() error {
	return e.err
}
