package check

import (
	"os"
	"path/filepath"
	"strings"
)

// isolatedWorktree is a detached HEAD checkout of a repository. Dirty files
// in the user's tree are not copied, so a later check cannot write them.
type isolatedWorktree struct {
	repo string
	root string
}

// withIsolatedWorktree adds a throwaway worktree of repoRoot's HEAD, calls
// fn, then removes that worktree. fn's error wins over a later remove
// error so a failed check is not reported as a cleanup failure.
func withIsolatedWorktree(repoRoot string, fn func(isolatedWorktree) error) (err error) {
	wt, err := addIsolatedWorktree(repoRoot)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := wt.close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return fn(wt)
}

func addIsolatedWorktree(repoRoot string) (isolatedWorktree, error) {
	repo, err := gitRepoRoot(repoRoot)
	if err != nil {
		return isolatedWorktree{}, err
	}
	dir, err := os.MkdirTemp("", "genguard-")
	if err != nil {
		return isolatedWorktree{}, err
	}
	// git worktree add refuses a path that already exists.
	if err := os.Remove(dir); err != nil {
		return isolatedWorktree{}, err
	}
	out, code, err := git(repo, "worktree", "add", "--detach", dir, "HEAD")
	if err != nil || code != 0 {
		_ = os.RemoveAll(dir)
		_, _, _ = git(repo, "worktree", "prune")
		return isolatedWorktree{}, isolateGitError("add", out, err)
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return isolatedWorktree{repo: repo, root: dir}, nil
}

func (w isolatedWorktree) close() error {
	if w.root == "" {
		return nil
	}
	out, code, err := git(w.repo, "worktree", "remove", "--force", w.root)
	if err == nil && code == 0 {
		return nil
	}
	_ = os.RemoveAll(w.root)
	_, _, _ = git(w.repo, "worktree", "prune")
	if _, statErr := os.Stat(w.root); os.IsNotExist(statErr) {
		return nil
	}
	return isolateGitError("remove", out, err)
}

// mapPath returns the same relative path inside the worktree. The user's
// api/genguard.yaml maps to that file at HEAD, which is the copy a check
// should load.
func (w isolatedWorktree) mapPath(path string) (string, error) {
	rel, err := relInsideRepo(w.repo, path)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return w.root, nil
	}
	return filepath.Join(w.root, rel), nil
}

func relInsideRepo(root, path string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if rel, ok := relInside(absRoot, absPath); ok {
		return rel, nil
	}
	resolvedRoot, rootErr := filepath.EvalSymlinks(absRoot)
	resolvedPath, pathErr := filepath.EvalSymlinks(absPath)
	if rootErr != nil || pathErr != nil {
		return "", newGenguardError("%s is not inside the repository", path)
	}
	if rel, ok := relInside(resolvedRoot, resolvedPath); ok {
		return rel, nil
	}
	return "", newGenguardError("%s is not inside the repository", path)
}

func relInside(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

func isolateGitError(op, out string, err error) error {
	detail := strings.TrimSpace(out)
	if detail == "" && err != nil {
		detail = err.Error()
	}
	if detail == "" {
		detail = "git worktree " + op + " failed"
	}
	return newGenguardError("git worktree %s: %s", op, detail)
}
