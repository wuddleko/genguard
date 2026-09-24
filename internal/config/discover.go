package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

var skipDirNames = map[string]struct{}{
	".git":         {},
	"vendor":       {},
	"node_modules": {},
}

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
	if err := rejectBothConfigNames(found); err != nil {
		return nil, err
	}
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
