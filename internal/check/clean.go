package check

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

func cleanOutputs(root string, group config.Group) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	gitDir := filepath.Join(absRoot, ".git")
	configYAML := filepath.Join(absRoot, "genguard.yaml")
	configYML := filepath.Join(absRoot, "genguard.yml")

	for _, spec := range group.Outputs {
		target, isDir, err := resolveCleanTarget(absRoot, spec)
		if err != nil {
			return err
		}
		if wouldRemove(target, gitDir) {
			return newGenguardError("clean refuses %q: mixed tree (would delete .git)", spec)
		}
		gitPath, err := treeContainsGit(target)
		if err != nil {
			return newGenguardError("clean %q: %s", spec, err.Error())
		}
		if gitPath != "" {
			rel, relErr := filepath.Rel(absRoot, gitPath)
			if relErr != nil {
				rel = gitPath
			}
			return newGenguardError("clean refuses %q: mixed tree (would delete %s)", spec, filepath.ToSlash(rel))
		}
		if wouldRemove(target, configYAML) || wouldRemove(target, configYML) {
			return newGenguardError("clean refuses %q: mixed tree (would delete the config file)", spec)
		}
		if err := removeCleanTarget(absRoot, target, isDir); err != nil {
			return newGenguardError("clean %q: %s", spec, err.Error())
		}
	}
	return nil
}

func resolveCleanTarget(root, spec string) (string, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false, newGenguardError("clean refuses an empty output path")
	}
	if isGlob(spec) {
		return "", false, newGenguardError("clean refuses glob output %q; use a directory path", spec)
	}
	if filepath.IsAbs(spec) {
		return "", false, newGenguardError("clean refuses absolute output %q", spec)
	}

	dirHint := strings.HasSuffix(spec, "/") || strings.HasSuffix(spec, string(filepath.Separator))
	cleaned := filepath.Clean(spec)
	if cleaned == "." {
		return "", false, newGenguardError("clean refuses %q", spec)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", false, newGenguardError("clean refuses %q", spec)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false, err
	}
	target := filepath.Clean(filepath.Join(absRoot, cleaned))
	rel, err := filepath.Rel(absRoot, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, newGenguardError("clean refuses %q", spec)
	}

	if err := refuseSymlinksInPath(absRoot, target); err != nil {
		return "", false, err
	}

	isDir := dirHint
	info, err := os.Lstat(target)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", false, newGenguardError("clean refuses symlink output %q", spec)
		}
		if info.IsDir() {
			isDir = true
		}
	}
	return target, isDir, nil
}

func pathComponents(rel string) ([]string, error) {
	parts := make([]string, 0)
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return nil, newGenguardError("clean refuses %q", rel)
		}
		parts = append(parts, part)
	}
	return parts, nil
}

func refuseSymlinksInPath(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	parts, err := pathComponents(rel)
	if err != nil {
		return err
	}
	cur := root
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return newGenguardError("clean refuses symlink in output path %q", filepath.Join(parts[:i+1]...))
		}
	}
	return nil
}

// treeContainsGit returns the absolute path of a .git file or directory that
// clean would delete under target. An empty path means there is none.
// Symlinks are not followed: clean unlinks them instead of descending, so a
// .git reachable only through a link is not deleted. The name is resolved by
// the filesystem, so a case-insensitive volume treats .GIT as .git.
func treeContainsGit(target string) (string, error) {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return gitEntryPath(target)
	}

	var found string
	err = filepath.WalkDir(target, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		gitPath, err := gitEntryPath(path)
		if err != nil {
			return err
		}
		if gitPath == "" {
			return nil
		}
		found = gitPath
		return fs.SkipAll
	})
	if err != nil {
		return "", err
	}
	return found, nil
}

// gitEntryPath returns the .git path in path's parent when path is that
// entry. Lookup is Lstat(".git"), so it follows the volume's case rules
// and does not follow a symlink named .git.
func gitEntryPath(path string) (string, error) {
	if !strings.EqualFold(filepath.Base(path), ".git") {
		return "", nil
	}
	probed := filepath.Join(filepath.Dir(path), ".git")
	if _, err := os.Lstat(probed); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return probed, nil
}

func wouldRemove(target, path string) bool {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absTarget, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func removeCleanTarget(root, target string, isDir bool) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	parts, err := pathComponents(rel)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return newGenguardError("clean refuses %q", rel)
	}
	return removePinned(root, parts, isDir)
}
