//go:build unix

package cli_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuddleko/genguard/tests/testutil"
)

func TestCLIIsolatedInterruptRemovesWorktree(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(tmp, "ready")
	script := "import sys, time\n" +
		"sys.stderr.write('line1\\n')\n" +
		"sys.stderr.flush()\n" +
		"open(sys.argv[1], 'w').close()\n" +
		"time.sleep(60)\n"
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts", "nap.py"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := "python3 scripts/nap.py " + shellQuote(ready)
	if _, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", command, "", nil); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "check", "--isolated", "--config", filepath.Join(root, "genguard.yaml"))
	cmd.Env = append(os.Environ(), "GENGUARD_RUN=1")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})

	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("command did not start\nstdout: %s\nstderr: %s", stdout.String(), stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}

	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	var err error
	select {
	case err = <-waited:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		err = <-waited
		t.Fatalf("interrupt did not stop the run: %v\nstderr: %s", err, stderr.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("err = %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "interrupted") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "line1") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	list, listErr := exec.Command("git", "-C", root, "worktree", "list", "--porcelain").CombinedOutput()
	if listErr != nil {
		t.Fatalf("worktree list: %v\n%s", listErr, list)
	}
	count := 0
	for _, line := range strings.Split(string(list), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("worktrees = %d\n%s", count, list)
	}
}
