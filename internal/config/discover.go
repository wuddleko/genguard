package config

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wuddleko/genguard/internal/gitx"
)

var skipDirNames = map[string]struct{}{
	".git":         {},
	"vendor":       {},
	"node_modules": {},
}

func SkipDir(name string) bool {
	_, skip := skipDirNames[name]
	return skip
}

func FindAll(ctx context.Context, repoRoot string, isolated bool) ([]string, error) {
	root, err := resolveStart(repoRoot)
	if err != nil {
		return nil, err
	}
	root, err = resolveWalkRoot(root)
	if err != nil {
		return nil, err
	}

	args := []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"}
	fallback := "git ls-files failed"
	if isolated {
		args = []string{"ls-tree", "-r", "-z", "--name-only", "HEAD"}
		fallback = "git ls-tree failed"
	}
	out, stderr, code, err := gitx.Run(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", gitx.Detail(out, fallback))
	}
	if err := listingFailure(stderr); err != nil {
		return nil, err
	}

	var found []string
	for _, name := range gitx.ParseNameList(out) {
		if !keepConfig(name) {
			continue
		}
		configPath := filepath.Join(root, filepath.FromSlash(name))
		if !isolated {
			if _, err := os.Lstat(configPath); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
		}
		found = append(found, configPath)
	}
	sort.Strings(found)
	if err := RejectBothConfigNames(found); err != nil {
		return nil, err
	}
	return found, nil
}

func listingFailure(stderr string) error {
	var lines []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || unreadableSkipDir(line) {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

func unreadableSkipDir(line string) bool {
	const prefix = "warning: could not open directory '"
	rest, ok := strings.CutPrefix(line, prefix)
	if !ok {
		return false
	}
	dir, _, ok := strings.Cut(rest, "'")
	if !ok {
		return false
	}
	dir = strings.Trim(filepath.ToSlash(dir), "/")
	for _, part := range strings.Split(dir, "/") {
		if SkipDir(part) {
			return true
		}
	}
	return false
}

func keepConfig(rel string) bool {
	rel = filepath.ToSlash(rel)
	if !IsConfigName(path.Base(rel)) {
		return false
	}
	dir := path.Dir(rel)
	if dir == "." {
		return true
	}
	for _, part := range strings.Split(dir, "/") {
		if SkipDir(part) {
			return false
		}
	}
	return true
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

func IsConfigName(name string) bool {
	for _, candidate := range configNames {
		if name == candidate {
			return true
		}
	}
	return false
}
