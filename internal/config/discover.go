package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Directories skipped at any depth while searching for configs.
// .gitignore is not consulted in v0.2.
var skipDirNames = map[string]struct{}{
	".git":         {},
	"vendor":       {},
	"node_modules": {},
}

// FindAll returns every genguard.yaml / genguard.yml under repoRoot,
// as absolute paths sorted lexicographically. Directories named .git,
// vendor, or node_modules are not searched. An empty result is not an
// error; the caller decides how to treat "no configs".
//
// repoRoot need not be a git repository. Empty repoRoot means the
// current working directory. A symlink root is followed; directory
// symlinks under it are not.
func FindAll(repoRoot string) ([]string, error) {
	root, err := resolveStart(repoRoot)
	if err != nil {
		return nil, err
	}

	root, err = resolveWalkRoot(root)
	if err != nil {
		return nil, err
	}

	var found []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(path, root, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isConfigName(d.Name()) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

func resolveWalkRoot(root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	walk := root
	if info.Mode()&os.ModeSymlink != 0 {
		walk, err = filepath.EvalSymlinks(root)
		if err != nil {
			return "", err
		}
		info, err = os.Stat(walk)
		if err != nil {
			return "", err
		}
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", root)
	}
	return walk, nil
}

func shouldSkipDir(path, root, name string) bool {
	if path == root {
		return false
	}
	_, skip := skipDirNames[name]
	return skip
}

func isConfigName(name string) bool {
	for _, candidate := range configNames {
		if name == candidate {
			return true
		}
	}
	return false
}
