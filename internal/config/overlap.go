package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/gitx"
	"github.com/wuddleko/genguard/internal/pathx"
)

type outputUse struct {
	config string
	group  string
	spec   string
	clean  string
	dir    bool
}

// errLeaveConfig means this config's outputs are not available yet. The caller
// reports the load itself.
var errLeaveConfig = errors.New("leave config")

var errOutsideRepo = errors.New("outside repository")

func rejectOutputOverlaps(configPath string, groups []Group) error {
	return rejectUses(outputUses(configPath, groups))
}

// RejectOutputOverlaps reports the first pair of output specs, across the
// working-tree configs, that can name the same path. A file that does not
// parse is left for the caller. A file that parses is included even when its
// own outputs overlap or its group names are duplicated.
func RejectOutputOverlaps(paths []string) error {
	return rejectAcross(paths, readWorktreeConfig)
}

// RejectCommittedOutputOverlaps is the same check for check --all --isolated.
// The specs are the HEAD blobs. Paths are the repository paths, so a config
// removed from the checkout still takes part.
func RejectCommittedOutputOverlaps(ctx context.Context, repoRoot string, paths []string) error {
	return rejectAcross(paths, func(path string) ([]byte, error) {
		return committedConfig(ctx, repoRoot, path)
	})
}

func rejectAcross(paths []string, read func(string) ([]byte, error)) error {
	var uses []outputUse
	for _, path := range paths {
		data, err := read(path)
		if errors.Is(err, errLeaveConfig) {
			continue
		}
		if err != nil {
			return err
		}
		cfg, err := configFromBytes(path, data)
		if err != nil {
			continue
		}
		uses = append(uses, outputUses(cfg.Path, cfg.Groups)...)
	}
	return rejectCrossUses(uses)
}

func readWorktreeConfig(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errLeaveConfig
	}
	return data, nil
}

func committedConfig(ctx context.Context, repoRoot, configPath string) ([]byte, error) {
	rel, err := repoRelative(repoRoot, configPath)
	if errors.Is(err, errOutsideRepo) {
		return nil, errLeaveConfig
	}
	if err != nil {
		return nil, err
	}
	out, _, code, err := gitx.Run(ctx, repoRoot, "--no-pager", "show", "--no-textconv", "HEAD:"+rel)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		detail := gitx.Detail(out, "git show failed")
		if blobMissing(detail) {
			return nil, errLeaveConfig
		}
		return nil, fmt.Errorf("%s", detail)
	}
	return []byte(out), nil
}

func blobMissing(detail string) bool {
	return strings.Contains(detail, "does not exist in") || strings.Contains(detail, "exists on disk, but not in")
}

func repoRelative(repoRoot, configPath string) (string, error) {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", err
	}
	absPath = filepath.Clean(absPath)
	roots := []string{repoRoot}
	if resolved, err := filepath.EvalSymlinks(repoRoot); err == nil {
		roots = append(roots, resolved)
	}
	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, ok := pathx.RelInside(filepath.Clean(absRoot), absPath)
		if ok && rel != "." {
			return filepath.ToSlash(rel), nil
		}
	}
	return "", fmt.Errorf("%s is not inside the repository: %w", configPath, errOutsideRepo)
}

func outputUses(configPath string, groups []Group) []outputUse {
	dir := filepath.Dir(configPath)
	uses := make([]outputUse, 0)
	for _, group := range groups {
		for _, spec := range group.Outputs {
			clean, isDir := outputClean(dir, spec)
			uses = append(uses, outputUse{
				config: configPath,
				group:  group.Name,
				spec:   spec,
				clean:  clean,
				dir:    isDir,
			})
		}
	}
	return uses
}

func outputClean(configDir, spec string) (string, bool) {
	isDir := strings.HasSuffix(spec, "/") || strings.HasSuffix(spec, string(filepath.Separator)) || pathx.IsGlob(spec)
	prefix := spec
	if isDir && pathx.IsGlob(spec) {
		prefix = globDir(spec)
	} else if isDir {
		prefix = strings.TrimRight(spec, `/\`)
		if prefix == "" {
			prefix = "."
		}
	}
	abs := filepath.Join(configDir, filepath.FromSlash(prefix))
	return filepath.ToSlash(filepath.Clean(abs)), isDir
}

func globDir(spec string) string {
	var dir []string
	for _, part := range strings.Split(filepath.ToSlash(spec), "/") {
		if part == "" || pathx.IsGlob(part) {
			break
		}
		dir = append(dir, part)
	}
	if len(dir) == 0 {
		return "."
	}
	return strings.Join(dir, "/")
}

func rejectUses(uses []outputUse) error {
	for i := range uses {
		for j := i + 1; j < len(uses); j++ {
			if outputOverlaps(uses[i], uses[j]) {
				return overlapError(uses[i], uses[j])
			}
		}
	}
	return nil
}

func rejectCrossUses(uses []outputUse) error {
	for i := range uses {
		for j := i + 1; j < len(uses); j++ {
			if uses[i].config == uses[j].config {
				continue
			}
			if outputOverlaps(uses[i], uses[j]) {
				return overlapError(uses[i], uses[j])
			}
		}
	}
	return nil
}

func outputOverlaps(a, b outputUse) bool {
	if a.dir && b.dir {
		return underPrefix(a.clean, b.clean) || underPrefix(b.clean, a.clean)
	}
	if a.dir {
		return underPrefix(b.clean, a.clean)
	}
	if b.dir {
		return underPrefix(a.clean, b.clean)
	}
	return a.clean == b.clean
}

func underPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

func overlapError(a, b outputUse) error {
	if a.config == b.config {
		return fmt.Errorf("outputs overlap: group %q %q and group %q %q", a.group, a.spec, b.group, b.spec)
	}
	return fmt.Errorf("outputs overlap: %s group %q %q and %s group %q %q", a.config, a.group, a.spec, b.config, b.group, b.spec)
}
