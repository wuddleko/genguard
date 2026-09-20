package check

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/regen/internal/config"
)

func cleanOutputs(root string, group config.Group) error {
	gitDir := filepath.Join(root, ".git")
	configYAML := filepath.Join(root, "regen.yaml")
	configYML := filepath.Join(root, "regen.yml")

	for _, spec := range group.Outputs {
		target, isDir, err := resolveCleanTarget(root, spec)
		if err != nil {
			return err
		}
		if wouldRemove(target, gitDir) {
			return newRegenError("clean refuses %q: mixed tree (would delete .git)", spec)
		}
		if _, err := os.Lstat(filepath.Join(target, ".git")); err == nil {
			return newRegenError("clean refuses %q: mixed tree (would delete .git)", spec)
		}
		if wouldRemove(target, configYAML) || wouldRemove(target, configYML) {
			return newRegenError("clean refuses %q: mixed tree (would delete the config file)", spec)
		}
		if err := removeCleanTarget(target, isDir); err != nil {
			return newRegenError("clean %q: %s", spec, err.Error())
		}
	}
	return nil
}

func resolveCleanTarget(root, spec string) (string, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false, newRegenError("clean refuses an empty output path")
	}
	if isGlob(spec) {
		return "", false, newRegenError("clean refuses glob output %q; use a directory path", spec)
	}
	if filepath.IsAbs(spec) {
		return "", false, newRegenError("clean refuses absolute output %q", spec)
	}

	dirHint := strings.HasSuffix(spec, "/") || strings.HasSuffix(spec, string(filepath.Separator))
	cleaned := filepath.Clean(spec)
	if cleaned == "." {
		return "", false, newRegenError("clean refuses %q", spec)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", false, newRegenError("clean refuses %q", spec)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false, err
	}
	target := filepath.Clean(filepath.Join(absRoot, cleaned))
	rel, err := filepath.Rel(absRoot, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, newRegenError("clean refuses %q", spec)
	}

	isDir := dirHint
	info, err := os.Lstat(target)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", false, newRegenError("clean refuses symlink output %q", spec)
		}
		if info.IsDir() {
			isDir = true
		}
	}
	return target, isDir, nil
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

func removeCleanTarget(target string, isDir bool) error {
	if isDir {
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		return os.MkdirAll(target, 0o755)
	}
	err := os.Remove(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
