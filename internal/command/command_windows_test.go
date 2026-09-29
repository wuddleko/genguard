//go:build windows

package command

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestRunCommandTimeoutKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	warmShell(t, root)

	pidPath := filepath.Join(root, "child.pid")
	command := pythonCommand(t, root, "nap.py", timeoutScript(pidPath))

	const limit = 8 * time.Second
	start := time.Now()
	tail, err := Run(context.Background(), root, command, nil, limit)
	if time.Since(start) >= 12*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err == nil || err.Error() != "command timed out after 8s" {
		t.Fatalf("err = %v", err)
	}
	if tail != "line1\n" {
		t.Fatalf("tail = %q, pid file: %s", tail, pidFileState(pidPath))
	}
	assertPidGone(t, pidPath)
}

func warmShell(t *testing.T, root string) {
	t.Helper()
	text, code, err := Capture(context.Background(), root, `python3 -c "import os, subprocess, sys; sys.stderr.write('ok'+chr(10)); sys.stderr.flush()"`, 45*time.Second)
	if err != nil || code != 0 || text != "ok\n" {
		t.Fatalf("warmup code=%d err=%v tail=%q", code, err, text)
	}
}

func pidFileState(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}

func TestApplySuspendedStartKeepsCmdLine(t *testing.T) {
	cmd := exec.Command("cmd.exe")
	const line = `cmd.exe /C echo hello world`
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	applySuspendedStart(cmd)
	if cmd.SysProcAttr.CmdLine != line {
		t.Fatalf("cmdline = %q", cmd.SysProcAttr.CmdLine)
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_SUSPENDED == 0 {
		t.Fatal("CREATE_SUSPENDED is unset")
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
