package check

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
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

func TestNoteCleanupKeepsTheFirstError(t *testing.T) {
	var result ConfigResult
	result.noteCleanup(nil)
	if result.cleanup != nil {
		t.Fatalf("cleanup = %v", result.cleanup)
	}
	result.noteCleanup(errors.New("first"))
	result.noteCleanup(errors.New("second"))
	if result.cleanup == nil || result.cleanup.Error() != "first" {
		t.Fatalf("cleanup = %v", result.cleanup)
	}
}

func TestSingleConfigRunRejectsBadPaths(t *testing.T) {
	_, err := SingleConfigRun(filepath.Join(t.TempDir(), "genguard.yaml"), ConfigResult{})
	if err == nil || !strings.Contains(err.Error(), "not a git work tree") {
		t.Fatalf("error = %v", err)
	}

	testutil.WithoutWorkingDirectory(t)
	_, err = SingleConfigRun("genguard.yaml", ConfigResult{})
	if err == nil {
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

func TestIsolateSinceRejectsUnusableRefs(t *testing.T) {
	dir := t.TempDir()
	_, err := isolateSince(dir, "HEAD")
	if err == nil || !strings.Contains(err.Error(), "not a git work tree") {
		t.Fatalf("non-repo: %v", err)
	}

	root := gitRepo(t)
	_, err = isolateSince(root, "not-a-ref")
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("bad ref: %v", err)
	}

	t.Run("quiet", func(t *testing.T) {
		installGitShim(t, "verify-quiet")
		_, err := isolateSince(root, "HEAD")
		if err == nil || !strings.Contains(err.Error(), "git rev-parse failed") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		installGitShim(t, "verify-empty")
		_, err := isolateSince(root, "HEAD")
		if err == nil || !strings.Contains(err.Error(), "empty revision") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestWithIsolatedCheckRejectsRelativePathWithoutCwd(t *testing.T) {
	testutil.WithoutWorkingDirectory(t)
	err := withIsolatedCheck("genguard.yaml", "", commandLog{}, func(config.Config, ConfigResult) error {
		t.Fatal("fn ran")
		return nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestIsolatedCheckMergeBaseFailure(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "keep.txt", "ok\n")
	writeTracked(t, root, "genguard.yaml", "groups:\n  - name: plain\n    command: \"true\"\n    outputs:\n      - keep.txt\n")
	installGitShim(t, "merge-base-quiet")
	_, err := CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "HEAD")
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
	t.Cleanup(func() { releaseWorktrees(t, root) })

	run := checkOneIsolated(filepath.Join(root, "genguard.yaml"), "", commandLog{})
	if run.Err != nil {
		t.Fatalf("Err = %v", run.Err)
	}
	if run.Result.cleanup == nil || !strings.Contains(run.Result.cleanup.Error(), "git worktree remove") {
		t.Fatalf("cleanup = %v", run.Result.cleanup)
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
	err := withIsolatedWorktree(root, func(isolatedWorktree) error {
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

func TestFindCommittedConfigsGitFailures(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "genguard.yaml", "groups: []\n")

	t.Run("quiet", func(t *testing.T) {
		installGitShim(t, "ls-tree-quiet")
		_, err := findCommittedConfigs(root)
		if err == nil || !strings.Contains(err.Error(), "git ls-tree failed") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing git", func(t *testing.T) {
		installGitShim(t, "drop-after-proxy")
		if _, _, err := git(root, "rev-parse", "--is-inside-work-tree"); err != nil {
			t.Fatal(err)
		}
		_, err := findCommittedConfigs(root)
		if err == nil {
			t.Fatal("git still on PATH after the shim removed itself")
		}
	})
}

func TestWithIsolatedCheckRejectsBadSince(t *testing.T) {
	root := gitRepo(t)
	err := withIsolatedCheck(filepath.Join(root, "genguard.yaml"), "not-a-ref", commandLog{}, func(config.Config, ConfigResult) error {
		t.Fatal("fn ran")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("error = %v", err)
	}
}

func TestIsolateSinceGitDisappears(t *testing.T) {
	root := gitRepo(t)
	installGitShim(t, "drop-on-toplevel")
	_, err := isolateSince(root, "HEAD")
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

func TestWithIsolatedCheckMapPathOutsideLexicalRoot(t *testing.T) {
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
	err := withIsolatedCheck(filepath.Join(link, "genguard.yaml"), "", commandLog{}, func(config.Config, ConfigResult) error {
		t.Fatal("fn ran")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "is not inside the repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestGitResultKeepsStderrWhenStartFails(t *testing.T) {
	out, code, err := gitResult("patch", "fatal: boom", errors.New("exec: git failed"))
	if err == nil || out != "fatal: boom" || code != -1 {
		t.Fatalf("out=%q code=%d err=%v", out, code, err)
	}
	out, code, err = gitResult("stdout", "", errors.New("exec: git failed"))
	if err == nil || out != "stdout" || code != -1 {
		t.Fatalf("out=%q code=%d err=%v", out, code, err)
	}
}

func TestRefuseFileInPathMissingAndFile(t *testing.T) {
	root := t.TempDir()
	if err := refuseFileInPath(filepath.Join(root, "missing")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := refuseFileInPath(file)
	if err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestRefuseFileInPathAllowsSymlinkParent(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := refuseFileInPath(link); err != nil {
		t.Fatal(err)
	}
}

func TestAnnotationPathsWhenLookupFails(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "genguard.yaml")
	text := FormatErrorAnnotation(outside, "boom")
	abs, err := filepath.Abs(outside)
	if err != nil {
		t.Fatal(err)
	}
	want := "::error file=" + escapeProperty(filepath.ToSlash(abs)) + "::boom\n"
	if text != want {
		t.Fatalf("outside = %q", text)
	}

	root := t.TempDir()
	testutil.WithoutWorkingDirectory(t)

	text = FormatErrorAnnotation("genguard.yaml", "boom")
	if text != "::error file=genguard.yaml::boom\n" {
		t.Fatalf("relative = %q", text)
	}

	t.Setenv("GITHUB_WORKSPACE", root)
	if got := workspaceFile("rel-root", "gen/a.go"); got != "gen/a.go" {
		t.Fatalf("repo abs = %q", got)
	}
	t.Setenv("GITHUB_WORKSPACE", "rel-workspace")
	if got := workspaceFile(root, "gen/a.go"); got != "gen/a.go" {
		t.Fatalf("workspace abs = %q", got)
	}
}

func releaseWorktrees(t *testing.T, repo string) {
	t.Helper()
	out, code, err := git(repo, "worktree", "list", "--porcelain")
	if err != nil || code != 0 {
		t.Errorf("worktree list: %v %s", err, out)
		return
	}
	repoAbs, err := filepath.Abs(repo)
	if err != nil {
		t.Error(err)
		return
	}
	for _, line := range strings.Split(out, "\n") {
		path, ok := strings.CutPrefix(line, "worktree ")
		if !ok {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil || abs == repoAbs {
			continue
		}
		if err := os.Chmod(path, 0o755); err != nil {
			t.Errorf("chmod %s: %v", path, err)
		}
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove %s: %v", path, err)
		}
	}
	_, _, _ = git(repo, "worktree", "prune")
}
