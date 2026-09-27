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

// ErrInterrupted is returned when the context is canceled while a command runs.
var ErrInterrupted = errors.New("interrupted")

// waitDelay bounds the wait for pipe copies after the child exits.
// A grandchild holding stdout would otherwise hang the run. Tests shorten it.
var waitDelay = 10 * time.Second

func Run(root, command string, onLine func(string), timeout time.Duration) (string, error) {
	return run(nil, root, command, onLine, timeout)
}

func RunContext(ctx context.Context, root, command string, onLine func(string), timeout time.Duration) (string, error) {
	return run(ctx, root, command, onLine, timeout)
}

// Capture runs command and returns its combined output, including on exit 0.
// The exit code is 0 on success. A timeout, an interrupt, or a failure to
// start returns a non-nil error and code 0.
func Capture(root, command string, timeout time.Duration) (string, int, error) {
	return execute(nil, root, command, nil, timeout)
}

func CaptureContext(ctx context.Context, root, command string, timeout time.Duration) (string, int, error) {
	return execute(ctx, root, command, nil, timeout)
}

func run(ctx context.Context, root, command string, onLine func(string), timeout time.Duration) (string, error) {
	text, code, err := execute(ctx, root, command, onLine, timeout)
	if err != nil || code != 0 {
		return text, err
	}
	return "", nil
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
	if timeoutFailure(err) {
		return ring.String(), 0, err
	}
	if err != nil && !started {
		return "", 0, &StartError{err: err}
	}
	// The child already exited 0. ErrWaitDelay means a grandchild still held a pipe.
	if err == nil {
		return ring.String(), 0, nil
	}

	exitCode := 1
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	tail := ring.String()
	if tail == "" {
		return "", exitCode, fmt.Errorf("command failed (exit %d): no output", exitCode)
	}
	return tail, exitCode, fmt.Errorf("command failed (exit %d)", exitCode)
}

// Output runs name with args and keeps stdout and stderr apart.
// A context whose Done channel is set starts a process group and returns
// ErrInterrupted when that context is canceled.
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

// waitCommand starts cmd and waits.
// WaitDelay bounds a pipe held open after the child exits. A timeout or a
// context with a Done channel also starts a process group, and that group's
// kill stays armed until Wait returns: a grandchild can keep the pipe open
// after the child has already exited.
// A context whose Done channel is nil is not a cancel. The command finishes,
// and the caller reads Err() itself.
func waitCommand(parent context.Context, cmd *exec.Cmd, timeout time.Duration) (bool, error) {
	cmd.WaitDelay = waitDelay
	if interrupted(parent) {
		return false, ErrInterrupted
	}
	if timeout <= 0 && !watchable(parent) {
		err := cmd.Run()
		if errors.Is(err, exec.ErrWaitDelay) && succeeded(cmd) {
			return cmd.Process != nil, nil
		}
		return cmd.Process != nil, err
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
		// A cancel that arrived with the exit still wins.
		if interrupted(parent) {
			group.stop(cmd)
			return true, ErrInterrupted
		}
		if errors.Is(err, exec.ErrWaitDelay) && succeeded(cmd) {
			// The child has exited. Stop the group so a grandchild holding a
			// pipe does not outlive WaitDelay.
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
		return true, timeoutError(timeout)
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

func timeoutError(limit time.Duration) error {
	return fmt.Errorf("command timed out after %s", limit)
}

func timeoutFailure(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "command timed out after ")
}

// StartError means the process never started. Capture reports exit code 0.
type StartError struct {
	err error
}

func (e *StartError) Error() string {
	return "command failed to start: " + e.err.Error()
}

func (e *StartError) Unwrap() error {
	return e.err
}

func errorsAsExit(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}
