//go:build unix

package check

import (
	"bytes"
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
	tail, err := runCommand(root, command, nil, "", 200*time.Millisecond)
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	var genguardErr *GenguardError
	if !errors.As(err, &genguardErr) || err.Error() != "command timed out after 200ms" {
		t.Fatalf("err = %v", err)
	}
	if tail != "line1\n" {
		t.Fatalf("tail = %q", tail)
	}
	assertPidGone(t, pidPath)
}

func TestRunCommandTimeoutWriterDropsTail(t *testing.T) {
	root := t.TempDir()
	command := pythonCommand(t, root, "nap.py", timeoutScript(filepath.Join(root, "child.pid")))
	var buf bytes.Buffer

	start := time.Now()
	tail, err := runCommand(root, command, &buf, "", 200*time.Millisecond)
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err == nil || err.Error() != "command timed out after 200ms" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if buf.String() != "line1\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRunCommandTimeoutWithNoOutput(t *testing.T) {
	root := t.TempDir()
	start := time.Now()
	tail, err := runCommand(root, "sleep 5", nil, "", 200*time.Millisecond)
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

func timeoutScript(pidPath string) string {
	return "import os, sys, time\n" +
		"fd = os.open(" + strconv.Quote(pidPath) + ", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)\n" +
		"os.write(fd, str(os.getpid()).encode())\n" +
		"os.close(fd)\n" +
		"sys.stderr.write('line1\\n')\n" +
		"sys.stderr.flush()\n" +
		"time.sleep(5)\n"
}

func pythonCommand(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return "python3 " + shSingle(path)
}

func shSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
