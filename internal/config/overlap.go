package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type outputUse struct {
	config string
	group  string
	spec   string
	pat    pattern
}

// ErrSkipConfig from Tree.ReadFile leaves that config out of the overlap
// check. The caller reports its load itself.
var ErrSkipConfig = errors.New("config not available")

// Tree is where RejectOutputOverlaps reads each config and asks whether an
// output path is a directory. The zero Tree is the working tree.
type Tree struct {
	ReadFile func(path string) ([]byte, error)
	IsDir    func(path string) bool
}

func rejectOutputOverlaps(configPath string, groups []Group) error {
	return rejectUses(outputUses(configPath, groups, worktreeIsDir), false)
}

// RejectOutputOverlaps reports the first pair of output specs, across the
// configs at paths, that can name the same path. A file that does not parse
// is left for the caller. A file that parses is included even when its own
// outputs overlap or its group names are duplicated.
func RejectOutputOverlaps(paths []string, tree Tree) error {
	read := tree.ReadFile
	if read == nil {
		read = readWorktreeConfig
	}
	isDir := tree.IsDir
	if isDir == nil {
		isDir = worktreeIsDir
	}
	var uses []outputUse
	for _, path := range paths {
		data, err := read(path)
		if errors.Is(err, ErrSkipConfig) {
			continue
		}
		if err != nil {
			return err
		}
		cfg, err := configFromBytes(path, data)
		if err != nil {
			continue
		}
		uses = append(uses, outputUses(cfg.Path, cfg.Groups, isDir)...)
	}
	return rejectUses(uses, true)
}

func readWorktreeConfig(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ErrSkipConfig
	}
	return data, nil
}

// worktreeIsDir does not follow a symlink: git does not read a pathspec
// through one, and clean refuses it.
func worktreeIsDir(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir()
}

func outputUses(configPath string, groups []Group, isDir func(string) bool) []outputUse {
	dir := filepath.Dir(configPath)
	uses := make([]outputUse, 0)
	for _, group := range groups {
		for _, spec := range group.Outputs {
			uses = append(uses, outputUse{
				config: configPath,
				group:  group.Name,
				spec:   spec,
				pat:    compileOutput(dir, spec, isDir),
			})
		}
	}
	return uses
}

// rejectUses reports the first overlapping pair. cross skips pairs from the
// same config, which LoadConfig has already checked.
func rejectUses(uses []outputUse, cross bool) error {
	for i := range uses {
		for j := i + 1; j < len(uses); j++ {
			if cross && uses[i].config == uses[j].config {
				continue
			}
			if patternsOverlap(uses[i].pat, uses[j].pat) {
				return overlapError(uses[i], uses[j])
			}
		}
	}
	return nil
}

func overlapError(a, b outputUse) error {
	if a.config == b.config {
		return fmt.Errorf("outputs overlap: group %q %q and group %q %q", a.group, a.spec, b.group, b.spec)
	}
	return fmt.Errorf("outputs overlap: %s group %q %q and %s group %q %q", a.config, a.group, a.spec, b.config, b.group, b.spec)
}
