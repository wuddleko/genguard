package check

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestErrorsAsExitRejectsOtherErrors(t *testing.T) {
	var target *exec.ExitError
	if errorsAsExit(errors.New("boom"), &target) {
		t.Fatal("non-exit error reported as ExitError")
	}
	if target != nil {
		t.Fatalf("target = %v", target)
	}
}

func TestCheckSinceRejectsNonRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := CheckSince(config.Config{Path: filepath.Join(dir, "genguard.yaml")}, "HEAD")
	if err == nil || !strings.Contains(err.Error(), "not a git work tree") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunSinceRejectsNonRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := RunSince(config.Config{Path: filepath.Join(dir, "genguard.yaml")}, "HEAD")
	if err == nil || !strings.Contains(err.Error(), "not a git work tree") {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckConfigNilDamage(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "generated/hello.txt", "hello\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: "true",
			Outputs: []string{"generated/hello.txt"},
		}},
	}
	result, err := checkConfig(cfg, "", nil, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || result.Groups[0].Status != GroupOK {
		t.Fatalf("groups = %+v", result.Groups)
	}
}

func TestCheckGroupAffectedGitError(t *testing.T) {
	root := gitRepo(t)
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
	result, err := checkConfig(cfg, "not-a-real-ref", map[string]pathSnap{}, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups[0].Status != GroupError || result.Groups[0].Err == nil {
		t.Fatalf("group = %+v", result.Groups[0])
	}
	if _, statErr := os.Stat(filepath.Join(root, "ran")); !os.IsNotExist(statErr) {
		t.Fatal("command ran after the affected check failed")
	}
}

func TestCheckCommandFailureThenDriftError(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "generated/hello.txt", "hello\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "broken",
			Command: `python3 -c "open('.git/HEAD','w').write('ref: refs/heads/missing\n'); raise SystemExit(1)"`,
			Outputs: []string{"generated/hello.txt"},
		}},
	}
	result, err := checkConfig(cfg, "", map[string]pathSnap{}, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil {
		t.Fatalf("group = %+v", group)
	}
	if !strings.Contains(group.Err.Error(), "fatal") {
		t.Fatalf("error = %v, want the git failure", group.Err)
	}
	if strings.Contains(group.Err.Error(), "command failed") {
		t.Fatalf("error = %v, want the drift error to replace the command error", group.Err)
	}
	if len(group.Drifts) != 0 {
		t.Fatalf("drifts = %+v", group.Drifts)
	}
}

func TestRecordCleanDamageIgnoresDriftError(t *testing.T) {
	damage := map[string]pathSnap{}
	recordCleanDamage(damage, t.TempDir(), config.Group{Outputs: []string{"missing.txt"}})
	if len(damage) != 0 {
		t.Fatalf("damage = %+v", damage)
	}
	snap := map[string]pathSnap{}
	recordCleanDamage(snap, t.TempDir(), config.Group{Outputs: []string{"missing.txt"}})
	if len(snap) != 0 {
		t.Fatalf("snap = %+v", snap)
	}
}

func TestSnapAndComparePaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "generated")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	fileSnap := snapPath(file)
	if fileSnap.missing || !fileSnap.hashed {
		t.Fatalf("file snap = %+v", fileSnap)
	}
	if !samePathSnap(file, fileSnap) {
		t.Fatal("file should match its snapshot")
	}
	dirSnap := snapPath(dir)
	if dirSnap.missing || dirSnap.hashed {
		t.Fatalf("dir snap = %+v", dirSnap)
	}
	if !samePathSnap(dir, dirSnap) {
		t.Fatal("directory should match an unhashed snapshot")
	}
	if samePathSnap(file, pathSnap{mode: fileSnap.mode, size: fileSnap.size}) {
		t.Fatal("regular file should not match an unhashed snapshot")
	}
	if samePathSnap(file, pathSnap{missing: true}) {
		t.Fatal("present file matched a missing snapshot")
	}
	if samePathSnap(filepath.Join(root, "gone.txt"), fileSnap) {
		t.Fatal("missing file matched a present snapshot")
	}
	if samePathSnap(file, pathSnap{mode: fileSnap.mode | 0o111, size: fileSnap.size, hashed: true, sum: fileSnap.sum}) {
		t.Fatal("mode mismatch matched")
	}
	if samePathSnap(file, pathSnap{mode: fileSnap.mode, size: fileSnap.size + 1, hashed: true, sum: fileSnap.sum}) {
		t.Fatal("size mismatch matched")
	}
	other := fileSnap
	other.sum[0] ^= 0xff
	if samePathSnap(file, other) {
		t.Fatal("content mismatch matched")
	}
}

func TestCallerRepoRootFallbacks(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	missing := filepath.Join(root, "missing")

	if got := callerRepoRoot(missing, root); got != filepath.Clean(root) {
		t.Fatalf("missing start = %q", got)
	}
	if got := callerRepoRoot(root, missing); got != filepath.Clean(missing) {
		t.Fatalf("missing git root = %q", got)
	}
	if got := callerRepoRoot(other, root); got != filepath.Clean(root) {
		t.Fatalf("outside = %q", got)
	}
}

func TestGitMissingFromPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()

	if err := RequireGitRepo(dir); err == nil {
		t.Fatal("RequireGitRepo")
	}
	out, code, err := git(dir, "status")
	if err == nil || code != -1 || out != "" {
		t.Fatalf("git() = %q, %d, %v", out, code, err)
	}
	if _, err := gitNames(dir, "status"); err == nil {
		t.Fatal("gitNames")
	}
	if _, err := gitDiffText(dir, "HEAD"); err == nil {
		t.Fatal("gitDiffText")
	}
	if _, err := gitPrefix(dir); err == nil {
		t.Fatal("gitPrefix")
	}
	if _, err := mergeBase(dir, "HEAD"); err == nil {
		t.Fatal("mergeBase")
	}
}

func TestMergeBaseBlankRef(t *testing.T) {
	_, err := mergeBase(t.TempDir(), "  ")
	if err == nil || !strings.Contains(err.Error(), "--since requires a ref") {
		t.Fatalf("error = %v", err)
	}
}

func TestGitQuietFailures(t *testing.T) {
	t.Run("exit 2", func(t *testing.T) {
		installGitShim(t, "exit-2-empty")
		out, code, err := git(t.TempDir(), "status")
		if err != nil || code != 2 || out != "" {
			t.Fatalf("git() = %q, %d, %v", out, code, err)
		}
	})
	t.Run("merge-base empty", func(t *testing.T) {
		installGitShim(t, "merge-base-empty")
		_, err := mergeBase(t.TempDir(), "HEAD")
		if err == nil || !strings.Contains(err.Error(), "empty merge-base") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("merge-base quiet", func(t *testing.T) {
		installGitShim(t, "merge-base-quiet")
		_, err := mergeBase(t.TempDir(), "HEAD")
		if err == nil || !strings.Contains(err.Error(), "git merge-base failed") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("show-prefix quiet", func(t *testing.T) {
		installGitShim(t, "show-prefix-quiet")
		_, err := gitPrefix(t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "git rev-parse --show-prefix failed") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("diff quiet", func(t *testing.T) {
		installGitShim(t, "diff-quiet")
		_, err := gitDiffText(t.TempDir(), "HEAD")
		if err == nil || !strings.Contains(err.Error(), "git diff failed") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestGitRepoRootShimFailures(t *testing.T) {
	root := gitRepo(t)

	t.Run("empty toplevel", func(t *testing.T) {
		installGitShim(t, "show-toplevel-empty")
		_, err := gitRepoRoot(root)
		if err == nil || !strings.Contains(err.Error(), "not a git work tree") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("toplevel fails", func(t *testing.T) {
		installGitShim(t, "show-toplevel-fail")
		_, err := gitRepoRoot(root)
		if err == nil || !strings.Contains(err.Error(), "not a git work tree") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("toplevel cannot start", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("cannot remove a running git shim")
		}
		installGitShim(t, "drop-after-proxy")
		_, err := gitRepoRoot(root)
		if err == nil || strings.Contains(err.Error(), "not a git work tree") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestDriftForGroupGitFailures(t *testing.T) {
	group := config.Group{Name: "greeting", Outputs: []string{"generated/hello.txt"}}

	t.Run("prefix fails", func(t *testing.T) {
		root := repoWithModifiedFile(t)
		installGitShim(t, "show-prefix-fail")
		_, err := DriftForGroup(root, group)
		if err == nil || !strings.Contains(err.Error(), "prefix failed") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("prefix is absolute", func(t *testing.T) {
		root := repoWithModifiedFile(t)
		installGitShim(t, "show-prefix-abs")
		_, err := DriftForGroup(root, group)
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("untracked list fails", func(t *testing.T) {
		root := gitRepo(t)
		writeTracked(t, root, "generated/hello.txt", "hello\n")
		installGitShim(t, "others-fail")
		_, err := DriftForGroup(root, group)
		if err == nil || !strings.Contains(err.Error(), "others") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestDriftDiffUntrackedRegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "new.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drifts := []Drift{{Path: "new.txt", Kind: "untracked"}}

	t.Run("empty diff", func(t *testing.T) {
		installGitShim(t, "diff-empty")
		got, err := DriftDiff(root, drifts)
		if err != nil {
			t.Fatal(err)
		}
		if got != "Untracked generated file: new.txt" {
			t.Fatalf("diff = %q", got)
		}
	})
	t.Run("diff fails", func(t *testing.T) {
		installGitShim(t, "diff-quiet")
		_, err := DriftDiff(root, drifts)
		if err == nil || !strings.Contains(err.Error(), "git diff failed") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestPathsWhenWorkingDirectoryIsGone(t *testing.T) {
	root := gitRepo(t)
	testutil.WithoutWorkingDirectory(t)

	if _, err := gitRepoRoot(""); err == nil {
		t.Fatal("gitRepoRoot empty")
	}
	if _, err := gitRepoRoot("."); err == nil {
		t.Fatal("gitRepoRoot dot")
	}
	if _, err := normalizeConfigPaths([]string{"genguard.yaml"}); err == nil {
		t.Fatal("normalizeConfigPaths")
	}
	if _, err := CheckAll(CheckAllOptions{RepoRoot: root, Paths: []string{"genguard.yaml"}}); err == nil {
		t.Fatal("CheckAll")
	}
}

func writeTracked(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitExec(t, root, "add", rel)
	gitExec(t, root, "commit", "-m", rel)
}

func repoWithModifiedFile(t *testing.T) string {
	t.Helper()
	root := gitRepo(t)
	writeTracked(t, root, "generated/hello.txt", "old\n")
	if err := os.WriteFile(filepath.Join(root, "generated", "hello.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestIsolatedRunKeepsCompletedCheck(t *testing.T) {
	removeErr := newGenguardError("git worktree remove: busy")
	result := ConfigResult{Groups: []GroupResult{{Name: "api", Status: GroupDrift, Drifts: []Drift{{
		Group: "api", Path: "out.txt", Kind: "modified",
	}}}}}
	result.noteCleanup(removeErr)
	run := isolatedRun("genguard.yaml", result, removeErr)
	if run.Err != nil {
		t.Fatalf("Err = %v, want the drift result", run.Err)
	}
	if len(run.Result.Groups) != 1 || run.Result.Groups[0].Status != GroupDrift {
		t.Fatalf("Result = %+v", run.Result)
	}
	if run.ExitCode() != 1 {
		t.Fatalf("exit = %d, want 1", run.ExitCode())
	}
	line := run.Result.FinalErrorLine()
	if !strings.Contains(line, "generated path drifted") || !strings.Contains(line, "git worktree remove: busy") {
		t.Fatalf("line = %q", line)
	}

	okResult := ConfigResult{Groups: []GroupResult{{Name: "api", Status: GroupOK}}}
	okResult.noteCleanup(removeErr)
	run = isolatedRun("genguard.yaml", okResult, removeErr)
	if run.Err != nil {
		t.Fatalf("Err = %v", run.Err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	report, err := FormatRunFailureReport(RunResult{Configs: []ConfigRun{run}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "api: OK") || !strings.Contains(report, "error (git worktree remove: busy)") || !strings.Contains(report, "error: git worktree remove: busy") {
		t.Fatalf("report = %q", report)
	}

	run = isolatedRun("genguard.yaml", ConfigResult{}, newGenguardError("git worktree add: failed"))
	if run.Err == nil || !strings.Contains(run.Err.Error(), "git worktree add") {
		t.Fatalf("Err = %v", run.Err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
}
