package check

import (
	"path/filepath"
	"sort"

	"github.com/wuddleko/genguard/internal/config"
)

// CheckAllOptions controls a multi-config run.
//
// RepoRoot is resolved to the git repository toplevel, whether it is
// empty (current working directory), a relative path, or a subdirectory
// of the repository. When Paths is empty, configs are discovered under
// that toplevel with config.FindAll. When Paths is set, those paths are
// checked as given and are not required to live under RepoRoot.
// RunResult.RepoRoot is always the resolved toplevel.
//
// CheckAll always runs configs sequentially on the shared working tree.
type CheckAllOptions struct {
	RepoRoot string
	Paths    []string
}

// CheckAll loads and checks each config, continuing after per-config
// failures. It returns a fatal error only when the run cannot start
// (the starting directory is not a git work tree, or discovery fails).
func CheckAll(opts CheckAllOptions) (RunResult, error) {
	repoRoot, err := gitRepoRoot(opts.RepoRoot)
	if err != nil {
		return RunResult{}, err
	}

	paths := opts.Paths
	if len(paths) == 0 {
		found, err := config.FindAll(repoRoot)
		if err != nil {
			return RunResult{}, err
		}
		paths = found
	}

	paths, err = normalizeConfigPaths(paths)
	if err != nil {
		return RunResult{}, err
	}

	run := RunResult{
		RepoRoot: repoRoot,
		Configs:  make([]ConfigRun, 0, len(paths)),
	}
	for _, path := range paths {
		run.Configs = append(run.Configs, checkOne(path))
	}
	return run, nil
}

func checkOne(path string) ConfigRun {
	cfgRun := ConfigRun{Path: path}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		cfgRun.Err = err
		return cfgRun
	}
	result, err := CheckConfig(cfg)
	if err != nil {
		cfgRun.Err = err
		return cfgRun
	}
	cfgRun.Result = result
	return cfgRun
}

func normalizeConfigPaths(paths []string) ([]string, error) {
	abs := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		resolved, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		abs = append(abs, resolved)
	}
	sort.Strings(abs)
	return abs, nil
}
