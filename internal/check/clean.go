package check

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

type cleanTarget struct {
	spec   string
	target string
	isDir  bool
}

func cleanOutputs(root, configPath string, group config.Group) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	gitDir := filepath.Join(absRoot, ".git")
	rootConfigs := rootConfigPaths(absRoot)
	configPaths := make([]string, len(rootConfigs), len(rootConfigs)+1)
	copy(configPaths, rootConfigs)
	if configPath != "" && !isListedPath(configPath, rootConfigs) {
		configPaths = append(configPaths, configPath)
	}

	planned := make([]cleanTarget, 0, len(group.Outputs))
	for _, spec := range group.Outputs {
		items, err := planCleanSpec(absRoot, spec)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := guardCleanTarget(absRoot, item, gitDir, configPaths); err != nil {
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
	target, isDir, err := resolveCleanPath(root, spec, true)
	if err != nil {
		return nil, err
	}
	return []cleanTarget{{spec: spec, target: target, isDir: isDir}}, nil
}

func planCleanGlob(root, spec string) ([]cleanTarget, error) {
	spec = strings.TrimSpace(spec)
	if err := validateGlobSpec(spec); err != nil {
		return nil, err
	}
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
	return refuseSymlinks(root, prefix)
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

func guardCleanTarget(absRoot string, item cleanTarget, gitDir string, configPaths []string) error {
	if wouldRemove(item.target, gitDir) {
		return newGenguardError("clean refuses %q: mixed tree (would delete .git)", item.spec)
	}
	gitPath, configHit, err := findProtected(item.target)
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
	if configHit != "" {
		return newGenguardError("clean refuses %q: mixed tree (would delete the config file)", item.spec)
	}
	for _, path := range configPaths {
		removes, err := removesConfig(item.target, path)
		if err != nil {
			return newGenguardError("clean %q: %s", item.spec, err.Error())
		}
		if removes {
			return newGenguardError("clean refuses %q: mixed tree (would delete the config file)", item.spec)
		}
	}
	return nil
}

func rootConfigPaths(absRoot string) []string {
	names := config.ConfigNames()
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(absRoot, name)
	}
	return paths
}

func isListedPath(path string, paths []string) bool {
	for _, existing := range paths {
		if existing == path {
			return true
		}
	}
	return false
}

func removesConfig(target, path string) (bool, error) {
	if wouldRemove(target, path) {
		return true, nil
	}
	same, err := sameExistingFile(target, path)
	if err != nil || same {
		return same, err
	}
	return directoryContains(target, path)
}

// directoryContains reports whether target is a directory whose removal would
// delete path. EvalSymlinks resolves a symlink to a file inside that directory.
// Ancestor identity follows symlinks. It does not use the hard-link filename
// rule in sameExistingFile.
func directoryContains(target, path string) (bool, error) {
	info, err := statPath(target, false)
	if err != nil || info == nil {
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if wouldRemove(target, resolved) {
		return true, nil
	}
	parent := resolved
	for {
		next := filepath.Dir(parent)
		if next == parent {
			return false, nil
		}
		parent = next
		same, err := sameStatFile(target, parent)
		if err != nil || same {
			return same, err
		}
	}
}

// sameExistingFile reports whether deleting target deletes path.
// Stat follows symlinks, so another spelling on a case-insensitive volume
// matches, and so does the target of the path that loaded the config.
// A second hard link is a different name; removing it leaves path in place.
func sameExistingFile(target, path string) (bool, error) {
	targetInfo, err := statPath(target, false)
	if err != nil || targetInfo == nil {
		return false, err
	}
	pathInfo, err := statPath(path, false)
	if err != nil || pathInfo == nil {
		return false, err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return sameStatFile(target, path)
	}
	if !os.SameFile(targetInfo, pathInfo) {
		return false, nil
	}
	if strings.EqualFold(filepath.Base(target), filepath.Base(path)) {
		return sameStatFile(filepath.Dir(target), filepath.Dir(path))
	}
	return false, nil
}

func sameStatFile(target, path string) (bool, error) {
	targetInfo, err := statPath(target, true)
	if err != nil || targetInfo == nil {
		return false, err
	}
	pathInfo, err := statPath(path, true)
	if err != nil || pathInfo == nil {
		return false, err
	}
	return os.SameFile(targetInfo, pathInfo), nil
}

func statPath(path string, follow bool) (os.FileInfo, error) {
	stat := os.Lstat
	if follow {
		stat = os.Stat
	}
	info, err := stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return info, nil
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
	if cleaned == "." || relEscapes(cleaned) {
		return "", false, newGenguardError("clean refuses %q", spec)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false, err
	}
	target := filepath.Clean(filepath.Join(absRoot, cleaned))
	rel, ok := relInside(absRoot, target)
	if !ok || rel == "." {
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

// Windows reports a path through a file as not-exist.
func refuseFileInPath(parent string) error {
	info, err := os.Lstat(parent)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	return newGenguardError("%s is not a directory", parent)
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
	return refuseSymlinks(root, parts)
}

func refuseSymlinks(root string, parts []string) error {
	cur := root
	for i, part := range parts {
		parent := cur
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return refuseFileInPath(parent)
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return newGenguardError("clean refuses symlink in output path %q", filepath.Join(parts[:i+1]...))
		}
	}
	return nil
}

func findProtected(target string) (string, string, error) {
	info, err := statPath(target, false)
	if err != nil || info == nil {
		return "", "", err
	}
	if !info.IsDir() {
		gitPath, err := gitEntryPath(target)
		if err != nil || gitPath != "" {
			return gitPath, "", err
		}
		configPath, err := configEntryPath(target)
		return "", configPath, err
	}

	var gitPath, configPath string
	err = filepath.WalkDir(target, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		foundGit, err := gitEntryPath(path)
		if err != nil {
			return err
		}
		if foundGit != "" {
			gitPath = foundGit
			return fs.SkipAll
		}
		if configPath != "" {
			return nil
		}
		configPath, err = configEntryPath(path)
		return err
	})
	if err != nil {
		return "", "", err
	}
	return gitPath, configPath, nil
}

func gitEntryPath(path string) (string, error) {
	_, probed, err := foldedEntry(path, ".git")
	return probed, err
}

func configEntryPath(path string) (string, error) {
	base := filepath.Base(path)
	name := ""
	for _, candidate := range config.ConfigNames() {
		if strings.EqualFold(base, candidate) {
			name = candidate
			break
		}
	}
	if name == "" {
		return "", nil
	}
	info, err := statPath(path, false)
	if err != nil || info == nil {
		return "", err
	}
	probeInfo, probed, err := foldedEntry(path, name)
	if err != nil || probeInfo == nil || !os.SameFile(info, probeInfo) {
		return "", err
	}
	return probed, nil
}

func foldedEntry(path, name string) (os.FileInfo, string, error) {
	if !strings.EqualFold(filepath.Base(path), name) {
		return nil, "", nil
	}
	probed := filepath.Join(filepath.Dir(path), name)
	info, err := statPath(probed, false)
	if err != nil || info == nil {
		return nil, "", err
	}
	return info, probed, nil
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
	_, ok := relInside(absTarget, absPath)
	return ok
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
