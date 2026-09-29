package check

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/gitx"
	"github.com/wuddleko/genguard/internal/pathx"
)

func requireGitRepo(log commandLog, root string) error {
	out, code, err := log.git(root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return err
	}
	if code != 0 || strings.TrimSpace(out) != "true" {
		return fmt.Errorf("%s is not a git work tree", root)
	}
	return nil
}

func RepoRoot(dir string) (string, error) {
	return gitRepoRoot(commandLog{}, dir)
}

func gitRepoRoot(log commandLog, dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = wd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := requireGitRepo(log, abs); err != nil {
		return "", err
	}
	out, code, err := log.git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(out)
	if code != 0 || root == "" {
		return "", fmt.Errorf("%s is not a git work tree", abs)
	}
	return callerRepoRoot(abs, root), nil
}

// rev-parse --show-toplevel resolves symlinks; keep the caller's spelling.
func callerRepoRoot(start, gitRoot string) string {
	resolvedStart, err := filepath.EvalSymlinks(start)
	if err != nil {
		return filepath.Clean(gitRoot)
	}
	resolvedRoot, err := filepath.EvalSymlinks(gitRoot)
	if err != nil {
		return filepath.Clean(gitRoot)
	}
	rel, ok := pathx.RelInside(resolvedRoot, resolvedStart)
	if !ok {
		return filepath.Clean(gitRoot)
	}
	if rel == "." {
		return filepath.Clean(start)
	}
	suffix := string(filepath.Separator) + rel
	if strings.HasSuffix(start, suffix) {
		return filepath.Clean(strings.TrimSuffix(start, suffix))
	}
	return filepath.Clean(gitRoot)
}

// verifyCommit resolves since to one commit. The revision follows
// --end-of-options, so git reads the ref text as a revision.
func verifyCommit(log commandLog, root, since string) (string, error) {
	out, code, err := log.git(root, "rev-parse", "--verify", "--end-of-options", since+"^{commit}")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("bad --since ref: %s", gitx.Detail(out, "git rev-parse failed"))
	}
	rev := strings.TrimSpace(out)
	if rev == "" {
		return "", errors.New("bad --since ref: empty revision")
	}
	return rev, nil
}

func mergeBase(log commandLog, root, since string) (string, error) {
	since = strings.TrimSpace(since)
	if since == "" {
		return "", errors.New("--since requires a ref")
	}
	rev, err := verifyCommit(log, root, since)
	if err != nil {
		return "", err
	}
	out, code, err := log.git(root, "merge-base", "HEAD", rev)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("bad --since ref: %s", gitx.Detail(out, "git merge-base failed"))
	}
	base := strings.TrimSpace(out)
	if base == "" {
		return "", errors.New("bad --since ref: empty merge-base")
	}
	return base, nil
}

func gitDiffNames(log commandLog, root, rev string, specs []string) ([]string, error) {
	args := append([]string{"-c", "diff.relative=false", "diff", "--no-renames", "--name-only", "-z", rev, "--"}, specs...)
	return gitNames(log, root, args...)
}

func gitUntracked(log commandLog, root string, specs []string) ([]string, error) {
	args := append([]string{"ls-files", "--others", "--exclude-standard", "--full-name", "-z", "--"}, specs...)
	return gitNames(log, root, args...)
}

func gitDiffText(root string, args ...string) (string, error) {
	full := append([]string{"-c", "diff.relative=false", "diff", "--no-color", "--no-ext-diff"}, args...)
	out, code, err := git(root, full...)
	if err != nil {
		return "", err
	}
	if code != 0 && code != 1 {
		return "", errors.New(gitx.Detail(out, "git diff failed"))
	}
	return out, nil
}

func gitNames(log commandLog, root string, args ...string) ([]string, error) {
	out, code, err := log.git(root, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New(gitx.Detail(out, "git failed"))
	}
	return gitx.ParseNameList(out), nil
}

func git(root string, args ...string) (string, int, error) {
	return commandLog{}.git(root, args...)
}

func (c commandLog) git(root string, args ...string) (string, int, error) {
	out, _, code, err := gitx.Run(c.ctx, root, args...)
	return out, code, err
}
