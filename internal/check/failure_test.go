package check

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/actions"
	"github.com/wuddleko/genguard/internal/command"
	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/internal/discover"
	"github.com/wuddleko/genguard/internal/gitx"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestRunGroupAffectedErrorSkipsCommand(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "in.txt", "ok\n")
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "sqlc",
			Command: `python3 -c "open('ran','w').close()"`,
			Inputs:  []string{"in.txt"},
			Outputs: []string{"out.txt"},
		}},
	}
	result, err := runConfig(cfg, "not-a-real-ref", commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil {
		t.Fatalf("group = %+v", group)
	}
	if _, statErr := os.Stat(filepath.Join(root, "ran")); !os.IsNotExist(statErr) {
		t.Fatal("command ran after the affected check failed")
	}
}

func TestNoteKeepsTheFirstError(t *testing.T) {
	var result ConfigResult
	result.note(nil)
	if result.extra != nil {
		t.Fatalf("extra = %v", result.extra)
	}
	result.note(errors.New("first"))
	result.note(errors.New("second"))
	if result.extra == nil || result.extra.Error() != "first" {
		t.Fatalf("extra = %v", result.extra)
	}

	var interrupted ConfigResult
	interrupted.note(command.ErrInterrupted)
	interrupted.note(errors.New("git worktree remove: busy"))
	if interrupted.extra == nil || interrupted.extra.Error() != "git worktree remove: busy" {
		t.Fatalf("extra = %v, want the removal error over the interrupt", interrupted.extra)
	}
}

func TestExecuteRejectsBadPaths(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genguard.yaml")
	writeFile(t, path, "groups:\n  - command: \"true\"\n    outputs: [out.txt]\n")
	_, err := Execute(Options{}, path)
	if err == nil || !strings.Contains(err.Error(), "not a git work tree") {
		t.Fatalf("error = %v", err)
	}

	testutil.WithoutWorkingDirectory(t)
	if _, err := Execute(Options{}, "genguard.yaml"); err == nil {
		t.Fatal("relative path")
	}
}

func TestCallerPathErrorRewritesOrWraps(t *testing.T) {
	if err := callerPathError(nil, "/mapped", "/caller"); err != nil {
		t.Fatalf("nil: %v", err)
	}

	mapped := "/tmp/worktree/genguard.yaml"
	caller := "/src/genguard.yaml"
	rewritten := callerPathError(&os.PathError{Op: "open", Path: mapped, Err: os.ErrNotExist}, mapped, caller)
	var pe *os.PathError
	if !errors.As(rewritten, &pe) || pe.Path != caller || pe.Op != "open" {
		t.Fatalf("path error = %#v", rewritten)
	}
	if !os.IsNotExist(rewritten) {
		t.Fatal("rewritten error lost not-exist")
	}

	plain := callerPathError(errors.New("parse failed"), mapped, caller)
	if plain == nil || !strings.Contains(plain.Error(), caller) || strings.Contains(plain.Error(), mapped) {
		t.Fatalf("plain = %v", plain)
	}
}

func TestMergeBaseRejectsUnusableRefs(t *testing.T) {
	root := gitRepo(t)
	_, err := mergeBase(commandLog{}, root, "not-a-ref")
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("bad ref: %v", err)
	}

	t.Run("quiet", func(t *testing.T) {
		installGitShim(t, "verify-quiet")
		_, err := mergeBase(commandLog{}, root, "HEAD")
		if err == nil || !strings.Contains(err.Error(), "git rev-parse failed") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		installGitShim(t, "verify-empty")
		_, err := mergeBase(commandLog{}, root, "HEAD")
		if err == nil || !strings.Contains(err.Error(), "empty revision") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCheckIsolatedRejectsRelativePathWithoutCwd(t *testing.T) {
	root := gitRepo(t)
	testutil.WithoutWorkingDirectory(t)
	if _, err := checkIsolated(commandLog{}, root, "genguard.yaml", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestIsolatedCheckMergeBaseFailure(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "keep.txt", "ok\n")
	writeTracked(t, root, "genguard.yaml", "groups:\n  - name: plain\n    command: \"true\"\n    outputs:\n      - keep.txt\n")
	installGitShim(t, "merge-base-quiet")
	_, err := Execute(Options{Isolated: true, Since: "HEAD"}, filepath.Join(root, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), "git merge-base failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckSinceIsolatedNotesFailedWorktreeRemoval(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode does not block removal")
	}
	root := gitRepo(t)
	writeTracked(t, root, "keep.txt", "ok\n")
	writeTracked(t, root, "genguard.yaml", "groups:\n  - name: lock\n    command: chmod 555 .\n    outputs:\n      - keep.txt\n")
	lockableTempDir(t)

	executed, err := Execute(Options{Isolated: true}, filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	run := executed.Configs[0]
	if run.Result.extra == nil || !strings.Contains(run.Result.extra.Error(), "git worktree remove") {
		t.Fatalf("extra = %v", run.Result.extra)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	if len(run.Result.Groups) != 1 || run.Result.Groups[0].Status != GroupOK {
		t.Fatalf("groups = %+v", run.Result.Groups)
	}
	if _, err := os.Stat(filepath.Join(root, "keep.txt")); err != nil {
		t.Fatalf("user tree: %v", err)
	}
}

func TestIsolatedWorktreeCloseEdges(t *testing.T) {
	if err := (isolatedWorktree{}).close(); err != nil {
		t.Fatal(err)
	}
	err := isolateGitError("add", "  ", nil)
	if err == nil || !strings.Contains(err.Error(), "git worktree add failed") {
		t.Fatalf("empty: %v", err)
	}
	err = isolateGitError("remove", "", errors.New("busy"))
	if err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("err: %v", err)
	}
}

func TestAddIsolatedWorktreeTempDirFailure(t *testing.T) {
	root := gitRepo(t)
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", file)
	err := withIsolatedWorktree(commandLog{}, root, func(isolatedWorktree) error {
		t.Fatal("fn ran")
		return nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRelInsideRepoFailurePaths(t *testing.T) {
	root := gitRepo(t)
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := relInsideRepo(root, missing)
	if err == nil || !strings.Contains(err.Error(), "is not inside the repository") {
		t.Fatalf("missing: %v", err)
	}

	testutil.WithoutWorkingDirectory(t)
	if _, err := relInsideRepo("rel", root); err == nil {
		t.Fatal("relative root")
	}
	if _, err := relInsideRepo(root, "rel"); err == nil {
		t.Fatal("relative path")
	}
}

func TestListConfigsGitFailures(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "genguard.yaml", "groups: []\n")

	t.Run("quiet", func(t *testing.T) {
		installGitShim(t, "ls-tree-quiet")
		_, err := discover.FindAll(context.Background(), root, true)
		if err == nil || !strings.Contains(err.Error(), "git ls-tree failed") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing git", func(t *testing.T) {
		installGitShim(t, "drop-after-proxy")
		if _, _, err := git(root, "rev-parse", "--is-inside-work-tree"); err != nil {
			t.Fatal(err)
		}
		_, err := discover.FindAll(context.Background(), root, true)
		if err == nil {
			t.Fatal("git still on PATH after the shim removed itself")
		}
	})
}

func TestIsolatedRejectsBadSince(t *testing.T) {
	root := gitRepo(t)
	_, err := Execute(Options{Isolated: true, Since: "not-a-ref"}, filepath.Join(root, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("error = %v", err)
	}
}

func TestIsolatedSinceGitDisappears(t *testing.T) {
	root := gitRepo(t)
	installGitShim(t, "drop-on-toplevel")
	_, err := Execute(Options{Isolated: true, Since: "HEAD"}, filepath.Join(root, "genguard.yaml"))
	if err == nil {
		t.Fatal("expected git to be missing for rev-parse --verify")
	}
}

func TestCloseRemovesDirectoryWhenGitRemoveFails(t *testing.T) {
	root := gitRepo(t)
	dir := filepath.Join(t.TempDir(), "not-a-worktree")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (isolatedWorktree{repo: root, root: dir}).close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("dir = %v", err)
	}
}

func TestRelInsideRepoFollowsSymlinkSpelling(t *testing.T) {
	root := gitRepo(t)
	file := filepath.Join(root, "f.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if resolved == root {
		t.Skip("repository path has no symlink to resolve")
	}
	rel, err := relInsideRepo(resolved, file)
	if err != nil {
		t.Fatal(err)
	}
	if rel != "f.txt" {
		t.Fatalf("rel = %q", rel)
	}
}

func TestIsolatedMapPathOutsideLexicalRoot(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "keep.txt", "ok\n")
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(sub, link); err != nil {
		t.Fatal(err)
	}
	_, err := Execute(Options{Isolated: true}, filepath.Join(link, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), "is not inside the repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestGitResultKeepsStderrWhenStartFails(t *testing.T) {
	out, code, err := gitx.Result("patch", "fatal: boom", errors.New("exec: git failed"))
	if err == nil || out != "fatal: boom" || code != -1 {
		t.Fatalf("out=%q code=%d err=%v", out, code, err)
	}
	out, code, err = gitx.Result("stdout", "", errors.New("exec: git failed"))
	if err == nil || out != "stdout" || code != -1 {
		t.Fatalf("out=%q code=%d err=%v", out, code, err)
	}
}

func TestAnnotationPathsWhenLookupFails(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "genguard.yaml")
	text := FormatErrorAnnotation(outside, "boom", actions.Env{})
	abs, err := filepath.Abs(outside)
	if err != nil {
		t.Fatal(err)
	}
	want := "::error file=" + actions.EscapeProperty(filepath.ToSlash(abs)) + "::boom\n"
	if text != want {
		t.Fatalf("outside = %q", text)
	}

	root := t.TempDir()
	testutil.WithoutWorkingDirectory(t)

	text = FormatErrorAnnotation("genguard.yaml", "boom", actions.Env{})
	if text != "::error file=genguard.yaml::boom\n" {
		t.Fatalf("relative = %q", text)
	}

	if got := actions.WorkspaceFile(root, "rel-root", "gen/a.go"); got != "gen/a.go" {
		t.Fatalf("repo abs = %q", got)
	}
	if got := actions.WorkspaceFile("rel-workspace", root, "gen/a.go"); got != "gen/a.go" {
		t.Fatalf("workspace abs = %q", got)
	}
}

// A failed git worktree remove still unregisters the worktree, so a read-only
// one cannot be found again through git. Isolated worktrees go under a
// per-test TMPDIR that is made writable before it is removed.
func lockableTempDir(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Cleanup(func() {
		_ = filepath.WalkDir(tmp, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(path, 0o755)
			}
			return nil
		})
	})
}
