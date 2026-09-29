//go:build unix

package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRunCommandTimeoutKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	pidPath := filepath.Join(root, "child.pid")
	command := pythonCommand(t, root, "nap.py", timeoutScript(pidPath))

	const limit = 3 * time.Second
	start := time.Now()
	tail, err := Run(context.Background(), root, command, nil, limit)
	if time.Since(start) >= 6*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err == nil || err.Error() != "command timed out after 3s" {
		t.Fatalf("err = %v", err)
	}
	if tail != "line1\n" {
		t.Fatalf("tail = %q", tail)
	}
	assertPidGone(t, pidPath)
}

func TestCaptureContextInterruptOutranksTimeout(t *testing.T) {
	root := t.TempDir()
	pidPath := filepath.Join(root, "child.pid")
	body := "import os, sys, time\n" +
		"sys.stderr.write('line1\\n')\n" +
		"sys.stderr.flush()\n" +
		"fd = os.open(" + strconv.Quote(pidPath) + ", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)\n" +
		"os.write(fd, str(os.getpid()).encode())\n" +
		"os.close(fd)\n" +
		"time.sleep(30)\n"
	command := pythonCommand(t, root, "nap.py", body)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	type got struct {
		text string
		code int
		err  error
	}
	ch := make(chan got, 1)
	start := time.Now()
	go func() {
		text, code, err := Capture(ctx, root, command, 30*time.Second)
		ch <- got{text, code, err}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(pidPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	var result got
	select {
	case result = <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("interrupt did not return")
	}
	if time.Since(start) >= 10*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if result.code != 0 || !errors.Is(result.err, ErrInterrupted) {
		t.Fatalf("code = %d err = %v tail = %q", result.code, result.err, result.text)
	}
	if result.err.Error() != "interrupted" {
		t.Fatalf("err = %v", result.err)
	}
	if !strings.Contains(result.text, "line1\n") {
		t.Fatalf("tail = %q", result.text)
	}
	assertPidGone(t, pidPath)
}

func TestRunCommandTimeoutWithNoOutput(t *testing.T) {
	root := t.TempDir()
	start := time.Now()
	tail, err := Run(context.Background(), root, "sleep 5", nil, 200*time.Millisecond)
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err == nil || err.Error() != "command timed out after 200ms" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
}

func assertPidGone(t *testing.T, pidPath string) {
	t.Helper()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("pid file = %q", data)
	}
	deadline := time.Now().Add(time.Second)
	var alive error
	for {
		alive = unix.Kill(pid, 0)
		if errors.Is(alive, unix.ESRCH) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid %d still alive: %v", pid, alive)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
