package check

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func RequireGitRepo(root string) error {
	return requireGitRepo(root)
}

func requireGitRepo(root string) error {
	out, code, err := git(root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return err
	}
	if code != 0 || strings.TrimSpace(out) != "true" {
		return newGenguardError("%s is not a git work tree", root)
	}
	return nil
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
	if err := requireGitRepo(abs); err != nil {
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

func mergeBase(root, since string) (string, error) {
	since = strings.TrimSpace(since)
	if since == "" {
		return "", newGenguardError("--since requires a ref")
	}
	out, code, err := git(root, "merge-base", "HEAD", since)
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

// --relative hides paths outside this directory, so names stay repo-root paths.
func gitDiffNames(root, rev string, specs []string) ([]string, error) {
	args := append([]string{"-c", "diff.relative=false", "diff", "--name-only", "-z", rev, "--"}, specs...)
	return gitNames(root, args...)
}

func gitUntracked(root string, specs []string) ([]string, error) {
	args := append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, specs...)
	return gitNames(root, args...)
}

func gitDiffText(root string, args ...string) (string, error) {
	full := append([]string{"-c", "diff.relative=false", "diff", "--no-color"}, args...)
	out, code, err := git(root, full...)
	if err != nil {
		return "", err
	}
	if code != 0 && code != 1 {
		return "", newGenguardError("%s", gitDetail(out, "git diff failed"))
	}
	return out, nil
}

func gitNames(root string, args ...string) ([]string, error) {
	out, code, err := git(root, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, newGenguardError("%s", gitDetail(out, "git failed"))
	}
	return parseGitNameList(out), nil
}

func gitPrefix(root string) (string, error) {
	out, code, err := git(root, "rev-parse", "--show-prefix")
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
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return gitResult(stdout.String(), stderr.String(), err)
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
