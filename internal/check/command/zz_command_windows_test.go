//go:build windows

package command

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestRunCommandTimeoutKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	pidPath := filepath.Join(root, "child.pid")
	command := pythonCommand(t, root, "nap.py", timeoutScript(pidPath))

	const limit = 3 * time.Second
	start := time.Now()
	tail, err := run(root, command, nil, limit)
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
	for {
		if !processRunning(pid) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("pid %d still alive", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func processRunning(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return true
	}
	return code == uint32(windows.STATUS_PENDING)
}
