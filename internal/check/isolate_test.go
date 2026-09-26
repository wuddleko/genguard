package check

import (
	"errors"
	"os"
	"path/filepath"
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

	err := withIsolatedWorktree(root, func(wt isolatedWorktree) error {
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

func TestWithIsolatedWorktreeRemovesOnSuccess(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "tracked.txt", "ok\n")
	var wtRoot string
	err := withIsolatedWorktree(root, func(wt isolatedWorktree) error {
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
	err := withIsolatedWorktree(root, func(wt isolatedWorktree) error {
		wtRoot = wt.root
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	assertWorktreeGone(t, root, wtRoot)
}

func TestWithIsolatedWorktreeAddFailureIsGenguardError(t *testing.T) {
	t.Run("not a git work tree", func(t *testing.T) {
		err := withIsolatedWorktree(t.TempDir(), func(isolatedWorktree) error {
			t.Fatal("fn ran")
			return nil
		})
		assertIsolateGenguardError(t, err, "not a git work tree")
	})
	t.Run("git worktree add", func(t *testing.T) {
		root := gitRepo(t)
		err := withIsolatedWorktree(root, func(isolatedWorktree) error {
			t.Fatal("fn ran")
			return nil
		})
		assertIsolateGenguardError(t, err, "git worktree add:")
		listed, listErr := gitWorktreeList(root)
		if listErr != nil {
			t.Fatal(listErr)
		}
		if strings.Count(listed, "worktree ") > 1 {
			t.Fatalf("leftover worktree:\n%s", listed)
		}
	})
}

func TestMapPathRefusesOutsideRepo(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "tracked.txt", "ok\n")
	err := withIsolatedWorktree(root, func(wt isolatedWorktree) error {
		_, err := wt.mapPath(t.TempDir())
		assertIsolateGenguardError(t, err, "is not inside the repository")
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

func TestWithIsolatedWorktreeFromSubdir(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "api/genguard.yaml", "nested\n")
	err := withIsolatedWorktree(filepath.Join(root, "api"), func(wt isolatedWorktree) error {
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

func assertIsolateGenguardError(t *testing.T, err error, want string) {
	t.Helper()
	var ge *GenguardError
	if !errors.As(err, &ge) {
		t.Fatalf("err = %v, want GenguardError", err)
	}
	if !strings.Contains(err.Error(), want) {
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

func gitWorktreeList(repo string) (string, error) {
	out, code, err := git(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", newGenguardError("git worktree list: %s", strings.TrimSpace(out))
	}
	return out, nil
}
