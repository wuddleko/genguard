package check

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitSeparatesStdoutAndStderr(t *testing.T) {
	root := gitRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(root, "generated", "changed.txt")
	same := filepath.Join(root, "generated", "same.txt")
	if err := os.WriteFile(changed, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(same, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, root, "add", "generated/changed.txt", "generated/same.txt")
	gitExec(t, root, "commit", "-m", "seed")
	gitExec(t, root, "config", "core.autocrlf", "true")
	if err := os.WriteFile(changed, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	untracked := filepath.Join(root, "generated", "new.txt")
	if err := os.WriteFile(untracked, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageGitFile(t, same)
	ageGitFile(t, changed)
	ageGitFile(t, untracked)

	t.Run("success drops a stderr warning", func(t *testing.T) {
		warn := gitStderr(t, root, "diff", "--name-only", "-z", "HEAD", "--", "generated/same.txt", "generated/changed.txt")
		if !strings.Contains(warn, "LF will be replaced by CRLF") {
			t.Fatalf("stderr = %q, want a CRLF warning", warn)
		}
		ageGitFile(t, same)
		ageGitFile(t, changed)
		out, code, err := git(root, "-c", "diff.relative=false", "diff", "--name-only", "-z", "HEAD", "--", "generated/same.txt", "generated/changed.txt")
		if err != nil || code != 0 {
			t.Fatalf("git() = %q, %d, %v", out, code, err)
		}
		if strings.Contains(out, "LF will be replaced by CRLF") {
			t.Fatalf("stdout = %q, warning leaked in", out)
		}
		if out != "generated/changed.txt\x00" {
			t.Fatalf("stdout = %q", out)
		}
	})

	t.Run("exit 1 keeps the patch and drops the warning", func(t *testing.T) {
		warn := gitStderrExit(t, root, 1, "diff", "--no-index", "--", os.DevNull, "generated/new.txt")
		if !strings.Contains(warn, "LF will be replaced by CRLF") {
			t.Fatalf("stderr = %q, want a CRLF warning", warn)
		}
		ageGitFile(t, untracked)
		out, code, err := git(root, "diff", "--no-index", "--", os.DevNull, "generated/new.txt")
		if err != nil || code != 1 {
			t.Fatalf("git() = %q, %d, %v", out, code, err)
		}
		if strings.Contains(out, "LF will be replaced by CRLF") {
			t.Fatalf("stdout = %q, warning leaked in", out)
		}
		if !strings.Contains(out, "+new") {
			t.Fatalf("stdout = %q, want the patch", out)
		}
	})

	t.Run("exit 1 with empty stdout returns the stderr error", func(t *testing.T) {
		out, code, err := git(root, "diff", "--no-index", "--", os.DevNull, "generated/missing.txt")
		if err != nil || code != 1 {
			t.Fatalf("git() = %q, %d, %v", out, code, err)
		}
		if !strings.Contains(out, "Could not access") {
			t.Fatalf("stdout = %q, want the git error", out)
		}
		out, code, err = git(root, "not-a-command")
		if err != nil || code != 1 {
			t.Fatalf("git() = %q, %d, %v", out, code, err)
		}
		if !strings.Contains(out, "not a git command") {
			t.Fatalf("stdout = %q, want the git error", out)
		}
	})

	t.Run("exit 1 with no output stays empty", func(t *testing.T) {
		out, code, err := git(root, "config", "--get", "no.such.key")
		if err != nil || code != 1 || out != "" {
			t.Fatalf("git() = %q, %d, %v", out, code, err)
		}
	})

	t.Run("other failures return stderr", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/missing\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, code, err := git(root, "diff", "--name-only", "-z", "HEAD")
		if err != nil || code != 128 {
			t.Fatalf("git() = %q, %d, %v", out, code, err)
		}
		if !strings.Contains(out, "fatal:") || !strings.Contains(out, "HEAD") || strings.Contains(out, "diff --git") {
			t.Fatalf("stdout = %q, want the fatal text", out)
		}
		if _, err := gitPrefix(t.TempDir()); err == nil || !strings.Contains(err.Error(), "fatal:") {
			t.Fatalf("gitPrefix error = %v, want git fatal text", err)
		}
	})
}

func TestGitNamesAndDiffTextKeepTheRightStream(t *testing.T) {
	root := gitRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(root, "generated", "changed.txt")
	if err := os.WriteFile(changed, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, root, "add", "generated/changed.txt")
	gitExec(t, root, "commit", "-m", "seed")
	gitExec(t, root, "config", "core.autocrlf", "true")
	if err := os.WriteFile(changed, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageGitFile(t, changed)

	out, code, err := git(root, "rev-parse", "--is-inside-work-tree")
	if err != nil || code != 0 || strings.TrimSpace(out) != "true" {
		t.Fatalf("git() = %q, %d, %v", out, code, err)
	}
	names, err := gitNames(root, "ls-files", "-z", "--", "no-such.txt")
	if err != nil || names != nil {
		t.Fatalf("names = %#v, %v", names, err)
	}
	names, err = gitNames(root, "-c", "diff.relative=false", "diff", "--name-only", "-z", "HEAD", "--", "generated/changed.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "generated/changed.txt" {
		t.Fatalf("names = %#v", names)
	}
	warn := gitStderrExit(t, root, 1, "diff", "--no-index", "--", os.DevNull, "generated/changed.txt")
	if !strings.Contains(warn, "LF will be replaced by CRLF") {
		t.Fatalf("stderr = %q, want a CRLF warning", warn)
	}
	ageGitFile(t, changed)
	text, err := gitDiffText(root, "--no-index", os.DevNull, "generated/changed.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "LF will be replaced by CRLF") || !strings.Contains(text, "+new") {
		t.Fatalf("diff = %q", text)
	}
	text, err = gitDiffText(root, "--no-index", os.DevNull, "generated/missing.txt")
	if err != nil || !strings.Contains(text, "Could not access") {
		t.Fatalf("diff = %q, %v", text, err)
	}

	if _, err := gitNames(root, "config", "--get", "no.such.key"); err == nil || !strings.Contains(err.Error(), "git failed") {
		t.Fatalf("error = %v, want git failed", err)
	}
	if _, err := gitNames(root, "not-a-command"); err == nil || !strings.Contains(err.Error(), "not a git command") {
		t.Fatalf("error = %v, want the git error", err)
	}

	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/missing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := gitNames(root, "diff", "--name-only", "-z", "HEAD"); err == nil || !strings.Contains(err.Error(), "fatal:") {
		t.Fatalf("error = %v, want git fatal text", err)
	}
	if _, err := gitDiffText(root, "HEAD", "--", "generated/changed.txt"); err == nil || !strings.Contains(err.Error(), "fatal: bad revision 'HEAD'") {
		t.Fatalf("error = %v, want git fatal text", err)
	}
}

func TestGitCommandNotFound(t *testing.T) {
	if os.Getenv("GENGUARD_GIT_NOT_FOUND") == "1" {
		out, code, err := git(t.TempDir(), "rev-parse", "--is-inside-work-tree")
		if err == nil || code != -1 || out != "" {
			t.Fatalf("git() = %q, %d, %v", out, code, err)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestGitCommandNotFound$")
	cmd.Env = append(os.Environ(), "GENGUARD_GIT_NOT_FOUND=1", "PATH="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child: %v\n%s", err, out)
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitExec(t, root, "init")
	gitExec(t, root, "config", "user.email", "test@example.com")
	gitExec(t, root, "config", "user.name", "Test")
	return root
}

func gitExec(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func gitStderr(t *testing.T, root string, args ...string) string {
	t.Helper()
	return gitStderrExit(t, root, 0, args...)
}

func gitStderrExit(t *testing.T, root string, want int, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if want == 0 {
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
		}
		return stderr.String()
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != want {
		t.Fatalf("git %s: %v, want exit %d\n%s", strings.Join(args, " "), err, want, stderr.String())
	}
	return stderr.String()
}

func ageGitFile(t *testing.T, path string) {
	t.Helper()
	past := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
}
