//go:build darwin

package command

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestCancelKillsProcessThatLeftTheGroup(t *testing.T) {
	root := t.TempDir()
	pidPath := filepath.Join(root, "child.pid")
	body := "import os, time\n" +
		"pid = os.fork()\n" +
		"if pid == 0:\n" +
		"    os.setpgid(0, 0)\n" +
		"    fd = os.open(" + strconv.Quote(pidPath) + ", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)\n" +
		"    os.write(fd, str(os.getpid()).encode())\n" +
		"    os.close(fd)\n" +
		"    time.sleep(30)\n" +
		"    os._exit(0)\n" +
		"time.sleep(30)\n"
	command := pythonCommand(t, root, "leave.py", body)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch := make(chan error, 1)
	go func() {
		_, err := Run(ctx, root, command, nil, 0)
		ch <- err
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

	select {
	case err := <-ch:
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("interrupt did not return")
	}
	assertPidGone(t, pidPath)
}
