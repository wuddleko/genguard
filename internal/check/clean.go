package check

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

// cleanTarget is one filesystem path clean will remove. spec is the output
// entry it came from, used in errors.
type cleanTarget struct {
	spec   string
	target string
	isDir  bool
}

func cleanOutputs(root string, group config.Group) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	gitDir := filepath.Join(absRoot, ".git")
	configYAML := filepath.Join(absRoot, "genguard.yaml")
	configYML := filepath.Join(absRoot, "genguard.yml")

	// Resolve every output before deleting any. A later refusal must not
	// leave earlier outputs wiped: the command never runs, and the next
	// group sees the broken tree.
	planned := make([]cleanTarget, 0, len(group.Outputs))
	for _, spec := range group.Outputs {
		items, err := planCleanSpec(absRoot, spec)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := guardCleanTarget(absRoot, item, gitDir, configYAML, configYML); err != nil {
				return err
			}
		}
		planned = append(planned, items...)
	}
	for _, item := range planned {
		if err := removeCleanTarget(absRoot, item.target, item.isDir); err != nil {
			return newGenguardError("clean %q: %s", item.spec, err.Error())
		}
	}
	return nil
}

func planCleanSpec(root, spec string) ([]cleanTarget, error) {
	if isGlob(spec) {
		return planCleanGlob(root, spec)
	}
	target, isDir, err := resolveCleanTarget(root, spec)
	if err != nil {
		return nil, err
	}
	return []cleanTarget{{spec: spec, target: target, isDir: isDir}}, nil
}

// planCleanGlob expands a git pathspec to the files drift would check:
// tracked files and untracked files that are not ignored. Those files are
// removed. The rest of the directory, including hand-written files, stays.
func planCleanGlob(root, spec string) ([]cleanTarget, error) {
	spec = strings.TrimSpace(spec)
	if err := validateGlobSpec(spec); err != nil {
		return nil, err
	}
	// Git does not list files through a directory symlink, so a prefix link
	// with no matches would skip the per-file checks and the command would
	// write through it.
	if err := refuseGlobPrefix(root, spec); err != nil {
		return nil, err
	}
	names, err := globCleanFiles(root, spec)
	if err != nil {
		return nil, newGenguardError("clean %q: %s", spec, err.Error())
	}
	items := make([]cleanTarget, 0, len(names))
	for _, name := range names {
		rel := filepath.ToSlash(filepath.Clean(name))
		target, isDir, err := resolveCleanPath(root, rel, false)
		if err != nil {
			return nil, err
		}
		if isDir {
			return nil, newGenguardError("clean refuses %q: glob matched directory %q", spec, rel)
		}
		items = append(items, cleanTarget{spec: spec, target: target, isDir: false})
	}
	return items, nil
}

// refuseGlobPrefix rejects a symlink in the literal directory prefix of a
// glob. generated/*_queries.sql.go checks generated. *_queries.sql.go has
// no directory prefix.
func refuseGlobPrefix(root, spec string) error {
	var prefix []string
	for _, part := range strings.Split(filepath.ToSlash(spec), "/") {
		if part == "" || part == "." {
			continue
		}
		if strings.ContainsAny(part, globChars) {
			break
		}
		prefix = append(prefix, part)
	}
	cur := root
	for i, part := range prefix {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return newGenguardError("clean refuses symlink in output path %q", filepath.Join(prefix[:i+1]...))
		}
	}
	return nil
}

func validateGlobSpec(spec string) error {
	if spec == "" {
		return newGenguardError("clean refuses an empty output path")
	}
	if filepath.IsAbs(spec) {
		return newGenguardError("clean refuses absolute output %q", spec)
	}
	for _, part := range strings.Split(filepath.ToSlash(spec), "/") {
		if part == ".." {
			return newGenguardError("clean refuses %q", spec)
		}
	}
	return nil
}

func globCleanFiles(root, spec string) ([]string, error) {
	tracked, err := gitNames(root, "ls-files", "-z", "--", spec)
	if err != nil {
		return nil, err
	}
	others, err := gitNames(root, "ls-files", "--others", "--exclude-standard", "-z", "--", spec)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(tracked)+len(others))
	files := make([]string, 0, len(tracked)+len(others))
	for _, name := range append(tracked, others...) {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		files = append(files, name)
	}
	return files, nil
}

func guardCleanTarget(absRoot string, item cleanTarget, gitDir, configYAML, configYML string) error {
	if wouldRemove(item.target, gitDir) {
		return newGenguardError("clean refuses %q: mixed tree (would delete .git)", item.spec)
	}
	gitPath, err := treeContainsGit(item.target)
	if err != nil {
		return newGenguardError("clean %q: %s", item.spec, err.Error())
	}
	if gitPath != "" {
		rel, relErr := filepath.Rel(absRoot, gitPath)
		if relErr != nil {
			rel = gitPath
		}
		return newGenguardError("clean refuses %q: mixed tree (would delete %s)", item.spec, filepath.ToSlash(rel))
	}
	if wouldRemove(item.target, configYAML) || wouldRemove(item.target, configYML) {
		return newGenguardError("clean refuses %q: mixed tree (would delete the config file)", item.spec)
	}
	return nil
}

func resolveCleanTarget(root, spec string) (string, bool, error) {
	return resolveCleanPath(root, spec, true)
}

func resolveCleanPath(root, spec string, rejectGlob bool) (string, bool, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", false, newGenguardError("clean refuses an empty output path")
	}
	if rejectGlob && isGlob(spec) {
		return "", false, newGenguardError("clean refuses glob output %q", spec)
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
