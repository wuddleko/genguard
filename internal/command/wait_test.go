package command

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCaptureStopsGrandchildHoldingStdout(t *testing.T) {
	for _, timeout := range []time.Duration{0, 2 * time.Second} {
		pidPath := captureHeldStdout(t, timeout)
		assertPidGone(t, pidPath)
	}
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
	text, code, err := Capture(context.Background(), root, command, timeout)
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
