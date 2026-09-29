package check

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

// Drift.Path is relative to the repository root.
type Drift struct {
	Group string
	Path  string
	Kind  string
}

// tree places a config directory in its repository.
type tree struct {
	dir    string
	repo   string
	prefix string
}

func newTree(repo, dir string) (tree, error) {
	rel, err := relInsideRepo(repo, dir)
	if err != nil {
		return tree{}, err
	}
	prefix := filepath.ToSlash(rel)
	if prefix == "." {
		prefix = ""
	}
	return tree{dir: dir, repo: repo, prefix: prefix}, nil
}

// repoPath turns a path relative to the config directory into a repository path.
func (t tree) repoPath(rel string) string {
	return path.Join(t.prefix, filepath.ToSlash(rel))
}

func (t tree) abs(repoPath string) string {
	return filepath.Join(t.repo, filepath.FromSlash(repoPath))
}

type pathSnap struct {
	missing bool
	mode    os.FileMode
	size    int64
	sum     [32]byte
	hashed  bool
}

func driftDiff(repoRoot string, drifts []Drift) (string, error) {
	parts := make([]string, 0, len(drifts))
	seen := make(map[string]struct{}, len(drifts))

	for _, item := range drifts {
		if _, ok := seen[item.Path]; ok {
			continue
		}
		seen[item.Path] = struct{}{}

		switch item.Kind {
		case "modified", "missing":
			diff, err := gitDiffText(repoRoot, "HEAD", "--", item.Path)
			if err != nil {
				return "", err
			}
			diff = strings.TrimSpace(diff)
			if diff != "" {
				parts = append(parts, diff)
			}
		case "untracked":
			target := filepath.Join(repoRoot, filepath.FromSlash(item.Path))
			info, err := os.Stat(target)
			if err != nil || !info.Mode().IsRegular() {
				parts = append(parts, fmt.Sprintf("Untracked generated file: %s", item.Path))
				continue
			}
			diff, err := gitDiffText(repoRoot, "--no-index", os.DevNull, item.Path)
			if err != nil {
				return "", err
			}
			diff = strings.TrimSpace(diff)
			if diff != "" {
				parts = append(parts, diff)
			} else {
				parts = append(parts, fmt.Sprintf("Untracked generated file: %s", item.Path))
			}
		}
	}

	return strings.Join(parts, "\n"), nil
}

func recordCleanDamage(log commandLog, damage map[string]pathSnap, t tree, group config.Group) error {
	found, err := driftForGroup(log, t, group)
	if err != nil {
		return err
	}
	for _, item := range found {
		damage[item.Path] = snapPath(t.abs(item.Path))
	}
	return nil
}

func omitUnchangedDamage(t tree, found []Drift, damage map[string]pathSnap) []Drift {
	if len(damage) == 0 || len(found) == 0 {
		return found
	}
	kept := make([]Drift, 0, len(found))
	for _, item := range found {
		snap, ok := damage[item.Path]
		if ok && samePathSnap(t.abs(item.Path), snap) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

func snapPath(abs string) pathSnap {
	info, err := os.Lstat(abs)
	if err != nil {
		return pathSnap{missing: true}
	}
	snap := pathSnap{mode: info.Mode(), size: info.Size()}
	if !info.Mode().IsRegular() {
		return snap
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return snap
	}
	snap.sum = sha256.Sum256(data)
	snap.hashed = true
	return snap
}

func samePathSnap(abs string, snap pathSnap) bool {
	info, err := os.Lstat(abs)
	if snap.missing {
		return os.IsNotExist(err)
	}
	if err != nil || info.Mode() != snap.mode || info.Size() != snap.size {
		return false
	}
	if !snap.hashed {
		return !info.Mode().IsRegular()
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	return sha256.Sum256(data) == snap.sum
}

func driftForGroup(log commandLog, t tree, group config.Group) ([]Drift, error) {
	found := make([]Drift, 0)
	seen := make(map[string]struct{})
	record := func(repoPath, kind string) {
		repoPath = path.Clean(repoPath)
		if _, ok := seen[repoPath]; ok {
			return
		}
		seen[repoPath] = struct{}{}
		found = append(found, Drift{Group: group.Name, Path: repoPath, Kind: kind})
	}

	modified, err := gitDiffNames(log, t.dir, "HEAD", group.Outputs)
	if err != nil {
		return nil, err
	}
	untracked, err := gitUntracked(log, t.dir, group.Outputs)
	if err != nil {
		return nil, err
	}

	for _, repoPath := range modified {
		kind := "modified"
		if _, err := os.Stat(t.abs(repoPath)); err != nil {
			kind = "missing"
		}
		record(repoPath, kind)
	}
	for _, repoPath := range untracked {
		record(repoPath, "untracked")
	}

	for _, spec := range group.Outputs {
		if literalOutputAbsent(t.dir, spec) {
			record(t.repoPath(spec), "missing")
		}
	}

	return found, nil
}

func groupAffected(log commandLog, root, base, configName string, group config.Group) (bool, error) {
	for _, spec := range group.Outputs {
		if literalOutputAbsent(root, spec) {
			return true, nil
		}
	}
	specs := make([]string, 0, len(group.Inputs)+len(group.Outputs)+1)
	specs = append(specs, group.Inputs...)
	specs = append(specs, group.Outputs...)
	if configName != "" {
		specs = append(specs, configName)
	}
	for _, rev := range []string{base, "HEAD"} {
		names, err := gitDiffNames(log, root, rev, specs)
		if err != nil || len(names) > 0 {
			return len(names) > 0, err
		}
	}
	untracked, err := gitUntracked(log, root, specs)
	return len(untracked) > 0, err
}

func literalOutputAbsent(root, spec string) bool {
	if parsed := config.ParseSpec(spec); parsed.Glob || parsed.Dir {
		return false
	}
	_, err := os.Stat(filepath.Join(root, spec))
	return err != nil
}
