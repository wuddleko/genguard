package check

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

const globChars = "*?[]"

type Drift struct {
	Group string
	Path  string
	Kind  string
}

type pathSnap struct {
	missing bool
	mode    os.FileMode
	size    int64
	sum     [32]byte
	hashed  bool
}

func DriftForGroup(root string, group config.Group) ([]Drift, error) {
	return driftForGroup(root, group)
}

func DriftDiff(root string, drifts []Drift) (string, error) {
	parts := make([]string, 0, len(drifts))
	seen := make(map[string]struct{}, len(drifts))

	for _, item := range drifts {
		if _, ok := seen[item.Path]; ok {
			continue
		}
		seen[item.Path] = struct{}{}

		switch item.Kind {
		case "modified", "missing":
			diff, err := gitDiffText(root, "HEAD", "--", item.Path)
			if err != nil {
				return "", err
			}
			diff = strings.TrimSpace(diff)
			if diff != "" {
				parts = append(parts, diff)
			}
		case "untracked":
			target := filepath.Join(root, item.Path)
			info, err := os.Stat(target)
			if err != nil || !info.Mode().IsRegular() {
				parts = append(parts, fmt.Sprintf("Untracked generated file: %s", item.Path))
				continue
			}
			diff, err := gitDiffText(root, "--no-index", os.DevNull, item.Path)
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

func recordCleanDamage(damage map[string]pathSnap, root string, group config.Group) {
	found, err := driftForGroup(root, group)
	if err != nil {
		return
	}
	recordFoundDamage(damage, root, found)
}

func recordFoundDamage(damage map[string]pathSnap, root string, found []Drift) {
	for _, item := range found {
		abs := absDriftPath(root, item.Path)
		damage[abs] = snapPath(abs)
	}
}

func omitUnchangedDamage(root string, found []Drift, damage map[string]pathSnap) []Drift {
	if len(damage) == 0 || len(found) == 0 {
		return found
	}
	kept := make([]Drift, 0, len(found))
	for _, item := range found {
		abs := absDriftPath(root, item.Path)
		snap, ok := damage[abs]
		if ok && samePathSnap(abs, snap) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

func dropRepairedDamage(damage map[string]pathSnap) {
	if len(damage) == 0 {
		return
	}
	for abs, snap := range damage {
		if !samePathSnap(abs, snap) {
			delete(damage, abs)
		}
	}
}

func absDriftPath(root, rel string) string {
	return filepath.Clean(filepath.Join(root, rel))
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

// driftPath is the config-relative spelling used as a drift key.
// Clean collapses ./hello.txt and foo/../hello.txt onto hello.txt, which is what git reports.
func driftPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func driftForGroup(root string, group config.Group) ([]Drift, error) {
	found := make([]Drift, 0)
	seen := make(map[string]struct{})
	record := func(path, kind string) {
		path = driftPath(path)
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		found = append(found, Drift{Group: group.Name, Path: path, Kind: kind})
	}

	modified, err := gitDiffNames(root, "HEAD", group.Outputs)
	if err != nil {
		return nil, err
	}
	if len(modified) > 0 {
		prefix, err := gitPrefix(root)
		if err != nil {
			return nil, err
		}
		for i, path := range modified {
			rel, err := configRelativeGitPath(prefix, path)
			if err != nil {
				return nil, err
			}
			modified[i] = rel
		}
	}
	untracked, err := gitUntracked(root, group.Outputs)
	if err != nil {
		return nil, err
	}

	for _, path := range modified {
		kind := "modified"
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			kind = "missing"
		}
		record(path, kind)
	}
	for _, path := range untracked {
		record(path, "untracked")
	}

	for _, spec := range group.Outputs {
		if literalOutputAbsent(root, spec) {
			record(spec, "missing")
		}
	}

	return found, nil
}

func groupAffected(root, base, configName string, group config.Group) (bool, error) {
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
		names, err := gitDiffNames(root, rev, specs)
		if err != nil || len(names) > 0 {
			return len(names) > 0, err
		}
	}
	untracked, err := gitUntracked(root, specs)
	return len(untracked) > 0, err
}

func literalOutputAbsent(root, spec string) bool {
	if isGlob(spec) || strings.HasSuffix(spec, "/") {
		return false
	}
	_, err := os.Stat(filepath.Join(root, spec))
	return err != nil
}

func isGlob(spec string) bool {
	return strings.ContainsAny(spec, globChars)
}
