package check

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/internal/pathx"
)

// checkIsolated checks the HEAD copy of path in a throwaway worktree of
// repoRoot. base comes from the caller's repository: refs such as @{u} mean
// nothing in the detached worktree.
func checkIsolated(log commandLog, repoRoot, path, base string) (ConfigResult, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ConfigResult{}, err
	}
	var result ConfigResult
	err = withIsolatedWorktree(log, repoRoot, func(wt isolatedWorktree) error {
		mapped, err := wt.mapPath(abs)
		if err != nil {
			return err
		}
		cfg, err := config.LoadConfig(mapped)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("%s is not in HEAD", path)
			}
			return callerPathError(err, mapped, path)
		}
		if result, err = groups(cfg, wt.root, base, log, ModeCheck); err != nil {
			return err
		}
		if drifts := result.AllDrifts(); len(drifts) > 0 {
			result.captureDriftDiff(driftDiff(wt.root, drifts))
		}
		return nil
	})
	if err != nil && len(result.Groups) > 0 {
		result.note(err)
	}
	return result, err
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
	repo   string
	parent string
	root   string
}

func withIsolatedWorktree(log commandLog, repoRoot string, fn func(isolatedWorktree) error) (err error) {
	wt, err := addIsolatedWorktree(log, repoRoot)
	if err != nil {
		return err
	}
	defer func() {
		cerr := wt.close()
		if cerr != nil && (err == nil || isInterrupt(err)) {
			err = cerr
		}
	}()
	return fn(wt)
}

func addIsolatedWorktree(log commandLog, repo string) (isolatedWorktree, error) {
	parent, err := os.MkdirTemp("", "genguard-")
	if err != nil {
		return isolatedWorktree{}, err
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		_ = os.RemoveAll(parent)
		return isolatedWorktree{}, err
	}
	// parent/wt does not exist yet. git worktree add creates it.
	// A missing hooks directory skips post-checkout, which would edit the new tree.
	dir := filepath.Join(parent, "wt")
	hooks := filepath.Join(parent, "no-hooks")
	out, code, err := log.git(repo, "-c", "core.hooksPath="+hooks, "worktree", "add", "--detach", dir, "HEAD")
	if err != nil || code != 0 {
		_ = os.RemoveAll(parent)
		_, _, _ = git(repo, "worktree", "prune")
		if isInterrupt(err) {
			return isolatedWorktree{}, err
		}
		return isolatedWorktree{}, isolateGitError("add", out, err)
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	if resolved, err := filepath.EvalSymlinks(parent); err == nil {
		parent = resolved
	}
	return isolatedWorktree{repo: repo, parent: parent, root: dir}, nil
}

func (w isolatedWorktree) close() error {
	if w.root == "" {
		return nil
	}
	out, code, err := git(w.repo, "worktree", "remove", "--force", w.root)
	if err != nil || code != 0 {
		_ = os.RemoveAll(w.root)
		_, _, _ = git(w.repo, "worktree", "prune")
		if _, statErr := os.Stat(w.root); !os.IsNotExist(statErr) {
			return isolateGitError("remove", out, err)
		}
	}
	if w.parent == "" {
		return nil
	}
	if rmErr := os.RemoveAll(w.parent); rmErr != nil {
		if _, statErr := os.Stat(w.parent); !os.IsNotExist(statErr) {
			return rmErr
		}
	}
	return nil
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
	if rel, ok := pathx.RelInsideResolved(root, path); ok {
		return rel, nil
	}
	return "", fmt.Errorf("%s is not inside the repository", path)
}

func isolateGitError(op, out string, err error) error {
	detail := strings.TrimSpace(out)
	if detail == "" && err != nil {
		detail = err.Error()
	}
	if detail == "" {
		detail = "git worktree " + op + " failed"
	}
	return fmt.Errorf("git worktree %s: %s", op, detail)
}
