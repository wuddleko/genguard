// Package discover lists genguard configs through git.
package discover

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/internal/gitx"
	"github.com/wuddleko/genguard/internal/pathx"
)

var skipDirNames = map[string]struct{}{
	".git":         {},
	"vendor":       {},
	"node_modules": {},
}

func skipDir(name string) bool {
	_, skip := skipDirNames[name]
	return skip
}

// FindAll lists the configs under repoRoot: the index and untracked files
// that are not ignored, or with isolated the HEAD tree.
func FindAll(ctx context.Context, repoRoot string, isolated bool) ([]string, error) {
	root, err := filepath.Abs(repoRoot)
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
		return nil, errors.New(gitx.Detail(out, fallback))
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
	if err := config.RejectBothConfigNames(found); err != nil {
		return nil, err
	}
	return found, nil
}

// Committed is the HEAD tree for config.RejectOutputOverlaps: configs and
// directories as committed. A config outside repoRoot or absent from HEAD is
// skipped.
func Committed(ctx context.Context, repoRoot string) config.Tree {
	var dirs map[string]bool
	return config.Tree{
		ReadFile: func(configPath string) ([]byte, error) {
			rel, ok := pathx.RelInsideResolved(repoRoot, configPath)
			if !ok || rel == "." {
				return nil, config.ErrSkipConfig
			}
			out, _, code, err := gitx.Run(ctx, repoRoot, "--no-pager", "show", "--no-textconv", "HEAD:"+filepath.ToSlash(rel))
			if err != nil {
				return nil, err
			}
			if code != 0 {
				detail := gitx.Detail(out, "git show failed")
				if strings.Contains(detail, "does not exist in") || strings.Contains(detail, "exists on disk, but not in") {
					return nil, config.ErrSkipConfig
				}
				return nil, errors.New(detail)
			}
			return []byte(out), nil
		},
		IsDir: func(path string) bool {
			rel, ok := pathx.RelInsideResolved(repoRoot, path)
			if !ok {
				return false
			}
			if rel == "." {
				return true
			}
			if dirs == nil {
				dirs = committedDirs(ctx, repoRoot)
			}
			return dirs[filepath.ToSlash(rel)]
		},
	}
}

// committedDirs lists the trees in HEAD. A symlink or a submodule is not one.
func committedDirs(ctx context.Context, repoRoot string) map[string]bool {
	dirs := map[string]bool{}
	out, _, code, err := gitx.Run(ctx, repoRoot, "ls-tree", "-r", "-d", "-z", "--name-only", "HEAD")
	if err != nil || code != 0 {
		return dirs
	}
	for _, name := range gitx.ParseNameList(out) {
		dirs[name] = true
	}
	return dirs
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
	return errors.New(strings.Join(lines, "\n"))
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
		if skipDir(part) {
			return true
		}
	}
	return false
}

func keepConfig(rel string) bool {
	rel = filepath.ToSlash(rel)
	if !config.IsConfigName(path.Base(rel)) {
		return false
	}
	dir := path.Dir(rel)
	if dir == "." {
		return true
	}
	for _, part := range strings.Split(dir, "/") {
		if skipDir(part) {
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
