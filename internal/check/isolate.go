package check

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

func CheckSinceIsolated(path, since string) (ConfigResult, error) {
	var result ConfigResult
	err := withIsolatedCheck(path, since, func(cfg config.Config, r ConfigResult) error {
		if drifts := r.AllDrifts(); len(drifts) > 0 {
			diff, diffErr := DriftDiff(cfg.Root(), drifts)
			r.captureDriftDiff(diff, diffErr)
		}
		result = r
		return nil
	})
	if err != nil && len(result.Groups) > 0 {
		result.noteCleanup(err)
	}
	return result, err
}

func withIsolatedCheck(path, since string, fn func(config.Config, ConfigResult) error) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// @{u} and HEAD@{1} are meaningless in the detached worktree.
	since, err = isolateSince(filepath.Dir(abs), since)
	if err != nil {
		return err
	}
	return withIsolatedWorktree(filepath.Dir(abs), func(wt isolatedWorktree) error {
		mapped, err := wt.mapPath(abs)
		if err != nil {
			return err
		}
		cfg, err := config.LoadConfig(mapped)
		if err != nil {
			if os.IsNotExist(err) {
				return newGenguardError("%s is not in HEAD", path)
			}
			return callerPathError(err, mapped, path)
		}
		result, err := CheckSince(cfg, since)
		if err != nil {
			return err
		}
		return fn(cfg, result)
	})
}

func isolateSince(dir, since string) (string, error) {
	since = strings.TrimSpace(since)
	if since == "" {
		return "", nil
	}
	root, err := gitRepoRoot(dir)
	if err != nil {
		return "", err
	}
	out, code, err := git(root, "rev-parse", "--verify", since+"^{commit}")
	if err != nil {
		return "", err
	}
	if code != 0 {
		detail := strings.TrimSpace(out)
		if detail == "" {
			detail = "git rev-parse failed"
		}
		return "", newGenguardError("bad --since ref: %s", detail)
	}
	rev := strings.TrimSpace(out)
	if rev == "" {
		return "", newGenguardError("bad --since ref: empty revision")
	}
	return rev, nil
}

func callerPathError(err error, mapped, path string) error {
	if err == nil {
		return nil
	}
	var pe *os.PathError
	if errors.As(err, &pe) && pe.Path == mapped {
		clone := *pe
		clone.Path = path
		return &clone
	}
	msg := err.Error()
	if mapped != "" && strings.Contains(msg, mapped) {
		return errors.New(strings.ReplaceAll(msg, mapped, path))
	}
	return fmt.Errorf("%s: %w", path, err)
}

type isolatedWorktree struct {
	repo string
	root string
}

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
	// A missing hooks directory skips post-checkout, which would edit the new tree.
	hooks := dir + "-hooks"
	out, code, err := git(repo, "-c", "core.hooksPath="+hooks, "worktree", "add", "--detach", dir, "HEAD")
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
	if err != nil || relEscapes(rel) {
		return "", false
	}
	return rel, true
}

func relEscapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
