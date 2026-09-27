package command

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCaptureReturnsWhenGrandchildHoldsStdout(t *testing.T) {
	pidPath := captureHeldStdout(t, 0)
	assertPidAlive(t, pidPath)
}

func TestCaptureProcessGroupStopsHeldStdout(t *testing.T) {
	pidPath := captureHeldStdout(t, 2*time.Second)
	assertPidGone(t, pidPath)
}

func captureHeldStdout(t *testing.T, timeout time.Duration) string {
	t.Helper()
	prev := waitDelay
	waitDelay = 200 * time.Millisecond
	t.Cleanup(func() { waitDelay = prev })

	root := t.TempDir()
	pidPath := filepath.Join(root, "child.pid")
	command := pythonCommand(t, root, "hold.py", holdStdoutScript(pidPath))
	t.Cleanup(func() { killPidFile(pidPath) })

	start := time.Now()
	text, code, err := Capture(root, command, timeout)
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err != nil || code != 0 {
		t.Fatalf("code = %d err = %v tail = %q", code, err, text)
	}
	if !strings.Contains(text, "done\n") {
		t.Fatalf("tail = %q", text)
	}
	return pidPath
}

func assertPidAlive(t *testing.T, pidPath string) {
	t.Helper()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("pid file = %q", data)
	}
	if !processRunning(pid) {
		t.Fatalf("pid %d is not running", pid)
	}
}

func killPidFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Kill()
}
