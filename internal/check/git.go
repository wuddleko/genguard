package check

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/check/command"
)

func RequireGitRepo(root string) error {
	return requireGitRepo(commandLog{}, root)
}

func requireGitRepo(log commandLog, root string) error {
	out, code, err := log.git(root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return err
	}
	if code != 0 || strings.TrimSpace(out) != "true" {
		return newGenguardError("%s is not a git work tree", root)
	}
	return nil
}

// RepoRoot is the git working tree that contains dir.
// An empty dir uses the current working directory.
func RepoRoot(dir string) (string, error) {
	return gitRepoRoot(dir)
}

func gitRepoRoot(dir string) (string, error) {
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
	if err := requireGitRepo(commandLog{}, abs); err != nil {
		return "", err
	}
	out, code, err := git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(out)
	if code != 0 || root == "" {
		return "", newGenguardError("%s is not a git work tree", abs)
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
	rel, ok := relInside(resolvedRoot, resolvedStart)
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

func mergeBase(log commandLog, root, since string) (string, error) {
	since = strings.TrimSpace(since)
	if since == "" {
		return "", newGenguardError("--since requires a ref")
	}
	out, code, err := log.git(root, "merge-base", "HEAD", since)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", newGenguardError("bad --since ref: %s", gitDetail(out, "git merge-base failed"))
	}
	base := strings.TrimSpace(out)
	if base == "" {
		return "", newGenguardError("bad --since ref: empty merge-base")
	}
	return base, nil
}

// --no-renames keeps a staged rename as a delete plus an add.
// --relative hides paths outside this directory, so names stay repo-root paths.
func gitDiffNames(log commandLog, root, rev string, specs []string) ([]string, error) {
	args := append([]string{"-c", "diff.relative=false", "diff", "--no-renames", "--name-only", "-z", rev, "--"}, specs...)
	return gitNames(log, root, args...)
}

func gitUntracked(log commandLog, root string, specs []string) ([]string, error) {
	args := append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, specs...)
	return gitNames(log, root, args...)
}

func gitDiffText(root string, args ...string) (string, error) {
	// --no-ext-diff ignores diff.external so the report stays a unified diff.
	full := append([]string{"-c", "diff.relative=false", "diff", "--no-color", "--no-ext-diff"}, args...)
	out, code, err := git(root, full...)
	if err != nil {
		return "", err
	}
	if code != 0 && code != 1 {
		return "", newGenguardError("%s", gitDetail(out, "git diff failed"))
	}
	return out, nil
}

func gitNames(log commandLog, root string, args ...string) ([]string, error) {
	out, code, err := log.git(root, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, newGenguardError("%s", gitDetail(out, "git failed"))
	}
	return parseGitNameList(out), nil
}

func gitPrefix(log commandLog, root string) (string, error) {
	out, code, err := log.git(root, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", newGenguardError("%s", gitDetail(out, "git rev-parse --show-prefix failed"))
	}
	return strings.TrimSpace(out), nil
}

func configRelativeGitPath(prefix, gitPath string) (string, error) {
	base := strings.TrimSuffix(prefix, "/")
	if base == "" {
		base = "."
	}
	rel, err := filepath.Rel(filepath.FromSlash(base), filepath.FromSlash(gitPath))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func parseGitNameList(out string) []string {
	if out == "" {
		return nil
	}
	parts := strings.Split(out, "\x00")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			names = append(names, part)
		}
	}
	return names
}

func gitDetail(out, fallback string) string {
	detail := strings.TrimSpace(out)
	if detail == "" {
		return fallback
	}
	return detail
}

func git(root string, args ...string) (string, int, error) {
	return commandLog{}.git(root, args...)
}

// git runs git for this run. A canceled context kills the process group and
// returns interrupted, including when git would otherwise exit 0 or look like a bad ref.
func (c commandLog) git(root string, args ...string) (string, int, error) {
	stdout, stderr, err := command.Output(c.ctx, "git", append([]string{"-C", root}, args...)...)
	if errors.Is(err, command.ErrInterrupted) {
		return "", 0, errInterrupted
	}
	return gitResult(stdout, stderr, err)
}

func gitResult(stdout, stderr string, err error) (string, int, error) {
	if err == nil {
		return stdout, 0, nil
	}
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		code := exitErr.ExitCode()
		// Exit 1 with a patch is a diff. A CRLF warning on stderr must not join it.
		if code == 1 {
			if strings.TrimSpace(stdout) == "" && strings.TrimSpace(stderr) != "" {
				return stderr, code, nil
			}
			return stdout, code, nil
		}
		if strings.TrimSpace(stderr) != "" {
			return stderr, code, nil
		}
		return stdout, code, nil
	}
	if strings.TrimSpace(stderr) != "" {
		return stderr, -1, err
	}
	return stdout, -1, err
}

func errorsAsExit(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}
