//go:build unix

package command

import (
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

	start := time.Now()
	tail, err := run(root, command, nil, 200*time.Millisecond)
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err == nil || err.Error() != "command timed out after 200ms" {
		t.Fatalf("err = %v", err)
	}
	if tail != "line1\n" {
		t.Fatalf("tail = %q", tail)
	}
	assertPidGone(t, pidPath)
}

func TestRunCommandTimeoutWithNoOutput(t *testing.T) {
	root := t.TempDir()
	start := time.Now()
	tail, err := run(root, "sleep 5", nil, 200*time.Millisecond)
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
