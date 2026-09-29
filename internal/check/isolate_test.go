package check

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWithIsolatedWorktreeOmitsDirtyFiles(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "api/genguard.yaml", "committed\n")
	writeTracked(t, root, "tracked.txt", "old\n")
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("dirt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := withIsolatedWorktree(commandLog{}, root, func(wt isolatedWorktree) error {
		got, err := os.ReadFile(filepath.Join(wt.root, "tracked.txt"))
		if err != nil {
			return err
		}
		if string(got) != "old\n" {
			t.Fatalf("tracked.txt = %q, want committed content", got)
		}
		if _, err := os.Stat(filepath.Join(wt.root, "untracked.txt")); !os.IsNotExist(err) {
			t.Fatalf("untracked.txt in worktree: %v", err)
		}
		mapped, err := wt.mapPath(filepath.Join(root, "api", "genguard.yaml"))
		if err != nil {
			return err
		}
		want := filepath.Join(wt.root, "api", "genguard.yaml")
		if mapped != want {
			t.Fatalf("mapPath = %q, want %q", mapped, want)
		}
		got, err = os.ReadFile(mapped)
		if err != nil {
			return err
		}
		if string(got) != "committed\n" {
			t.Fatalf("mapped config = %q", got)
		}
		user, err := os.ReadFile(filepath.Join(root, "tracked.txt"))
		if err != nil {
			return err
		}
		if string(user) != "new\n" {
			t.Fatalf("user tracked.txt = %q, helper mutated the checkout", user)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIsolatedWorktreeUsesPrivateParent(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "tracked.txt", "ok\n")
	logPath := filepath.Join(t.TempDir(), "git-args")
	installGitShim(t, "record")
	t.Setenv("GENGUARD_GIT_ARGS", logPath)

	var parent string
	err := withIsolatedWorktree(commandLog{}, root, func(wt isolatedWorktree) error {
		worktree, hooks := isolatedAddPaths(t, recordedGitArgs(t, logPath))
		if filepath.Base(wt.root) != "wt" || filepath.Base(hooks) != "no-hooks" {
			t.Fatalf("worktree = %s, hooks = %s", wt.root, hooks)
		}
		if _, err := os.Stat(wt.root); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(hooks); !os.IsNotExist(err) {
			t.Fatalf("hooks path = %v", err)
		}
		parent = filepath.Dir(wt.root)
		if !samePath(parent, filepath.Dir(hooks)) || !samePath(wt.root, worktree) {
			t.Fatalf("worktree %s parent %s, hooks %s", wt.root, parent, hooks)
		}
		info, err := os.Stat(parent)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Fatalf("parent mode = %o", info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("parent still exists: %v", err)
	}
}

func TestIsolatedWorktreeAddFailureRemovesParent(t *testing.T) {
	root := gitRepo(t)
	logPath := filepath.Join(t.TempDir(), "git-args")
	installGitShim(t, "record")
	t.Setenv("GENGUARD_GIT_ARGS", logPath)

	err := withIsolatedWorktree(commandLog{}, root, func(isolatedWorktree) error {
		t.Fatal("fn ran")
		return nil
	})
	assertIsolateError(t, err, "git worktree add:")
	_, hooks := isolatedAddPaths(t, recordedGitArgs(t, logPath))
	parent := filepath.Dir(hooks)
	if _, statErr := os.Stat(parent); !os.IsNotExist(statErr) {
		t.Fatalf("parent = %v", statErr)
	}
}

func TestWithIsolatedWorktreeRemovesOnSuccess(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "tracked.txt", "ok\n")
	var wtRoot string
	err := withIsolatedWorktree(commandLog{}, root, func(wt isolatedWorktree) error {
		wtRoot = wt.root
		if _, err := os.Stat(wt.root); err != nil {
			t.Fatalf("worktree missing during fn: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertWorktreeGone(t, root, wtRoot)
}

func TestWithIsolatedWorktreeRemovesOnFnError(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "tracked.txt", "ok\n")
	boom := errors.New("boom")
	var wtRoot string
	err := withIsolatedWorktree(commandLog{}, root, func(wt isolatedWorktree) error {
		wtRoot = wt.root
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	assertWorktreeGone(t, root, wtRoot)
}

func TestWithIsolatedWorktreeAddFailure(t *testing.T) {
	t.Run("not a repository", func(t *testing.T) {
		err := withIsolatedWorktree(commandLog{}, t.TempDir(), func(isolatedWorktree) error {
			t.Fatal("fn ran")
			return nil
		})
		assertIsolateError(t, err, "git worktree add:")
	})
	t.Run("git worktree add", func(t *testing.T) {
		root := gitRepo(t)
		err := withIsolatedWorktree(commandLog{}, root, func(isolatedWorktree) error {
			t.Fatal("fn ran")
			return nil
		})
		assertIsolateError(t, err, "git worktree add:")
		listed, listErr := gitWorktreeList(root)
		if listErr != nil {
			t.Fatal(listErr)
		}
		if strings.Count(listed, "worktree ") > 1 {
			t.Fatalf("leftover worktree:\n%s", listed)
		}
	})
}

func TestIsolatedAllOverlapReadsDirectoriesFromHead(t *testing.T) {
	setup := func(t *testing.T, headDir bool) string {
		root := gitRepo(t)
		if headDir {
			writeTracked(t, root, "api/gen/y.txt", "y\n")
		} else {
			writeTracked(t, root, "api/gen", "f\n")
		}
		writeTracked(t, root, "genguard.yaml", "groups:\n  - name: a\n    command: \"true\"\n    outputs: [api/gen]\n")
		writeTracked(t, root, "api/genguard.yaml", "groups:\n  - name: b\n    command: \"true\"\n    outputs: [gen/x.go]\n")
		return root
	}
	t.Run("directory removed from the checkout", func(t *testing.T) {
		root := setup(t, true)
		if err := os.RemoveAll(filepath.Join(root, "api", "gen")); err != nil {
			t.Fatal(err)
		}
		_, err := ExecuteAll(Options{RepoRoot: root, Isolated: true})
		if err == nil || !strings.Contains(err.Error(), "outputs overlap") {
			t.Fatalf("err = %v, want the overlap HEAD has", err)
		}
	})
	t.Run("directory only in the checkout", func(t *testing.T) {
		root := setup(t, false)
		gen := filepath.Join(root, "api", "gen")
		if err := os.Remove(gen); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(gen, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := ExecuteAll(Options{RepoRoot: root, Isolated: true}); err != nil {
			t.Fatalf("err = %v, want HEAD's file to overlap nothing", err)
		}
	})
}

func TestMapPathRefusesOutsideRepo(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "tracked.txt", "ok\n")
	err := withIsolatedWorktree(commandLog{}, root, func(wt isolatedWorktree) error {
		_, err := wt.mapPath(t.TempDir())
		assertIsolateError(t, err, "is not inside the repository")
		mapped, err := wt.mapPath(root)
		if err != nil {
			return err
		}
		if mapped != wt.root {
			t.Fatalf("mapPath(repo) = %q, want %q", mapped, wt.root)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMapPathNestedConfig(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "api/genguard.yaml", "nested\n")
	err := withIsolatedWorktree(commandLog{}, root, func(wt isolatedWorktree) error {
		mapped, err := wt.mapPath(filepath.Join(root, "api", "genguard.yaml"))
		if err != nil {
			return err
		}
		got, err := os.ReadFile(mapped)
		if err != nil {
			return err
		}
		if string(got) != "nested\n" {
			t.Fatalf("mapped = %q", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertIsolateError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %q, want substring %q", err, want)
	}
}

func assertWorktreeGone(t *testing.T, repo, wtRoot string) {
	t.Helper()
	if wtRoot == "" {
		t.Fatal("worktree path was empty")
	}
	if _, err := os.Stat(wtRoot); !os.IsNotExist(err) {
		t.Fatalf("worktree %s still exists: %v", wtRoot, err)
	}
	listed, err := gitWorktreeList(repo)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(listed, wtRoot) || strings.Contains(listed, filepath.ToSlash(wtRoot)) {
		t.Fatalf("git still lists %s:\n%s", wtRoot, listed)
	}
}

func isolatedAddPaths(t *testing.T, invocations [][]string) (worktree, hooks string) {
	t.Helper()
	for _, args := range invocations {
		if !containsArg(args, "worktree") || !containsArg(args, "add") {
			continue
		}
		for i, arg := range args {
			if arg == "--detach" && i+1 < len(args) {
				worktree = args[i+1]
			}
			const prefix = "core.hooksPath="
			if strings.HasPrefix(arg, prefix) {
				hooks = strings.TrimPrefix(arg, prefix)
			}
		}
	}
	if worktree == "" || hooks == "" {
		t.Fatalf("worktree add args = %#v", invocations)
	}
	return worktree, hooks
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	ai, err1 := os.Stat(a)
	bi, err2 := os.Stat(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

func gitWorktreeList(repo string) (string, error) {
	out, code, err := git(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("git worktree list: %s", strings.TrimSpace(out))
	}
	return out, nil
}
