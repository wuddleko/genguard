package check

import (
	"path/filepath"
	"sort"

	"github.com/wuddleko/genguard/internal/command"
	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/internal/discover"
)

// ExecuteAll runs every config under the repository, or opts.Paths. The error
// is set when nothing ran.
func ExecuteAll(opts Options) (RunResult, error) {
	if opts.Mode == ModeRun && opts.Isolated {
		return RunResult{}, errRunIsolated
	}
	log := commandLog{ctx: opts.Context, env: opts.Env}
	repoRoot, paths, base, err := discoverConfigs(log, opts)
	if err != nil {
		return RunResult{}, err
	}
	run := RunResult{RepoRoot: repoRoot, All: true, Configs: make([]ConfigRun, 0, len(paths))}
	for _, path := range paths {
		if log.canceled() {
			break
		}
		cfgLog := log
		if opts.Log != nil {
			cfgLog.w = opts.Log
			cfgLog.quiet = opts.Quiet
			cfgLog.prefix = displayConfigPath(repoRoot, path) + ": "
		}
		run.Configs = append(run.Configs, runListed(opts, cfgLog, repoRoot, path, base))
	}
	if !log.canceled() {
		return run, nil
	}
	if len(run.Configs) == 0 {
		return run, command.ErrInterrupted
	}
	if last := &run.Configs[len(run.Configs)-1]; !last.hasInterrupt() {
		last.Result.note(command.ErrInterrupted)
	}
	return run, nil
}

func runListed(opts Options, log commandLog, repoRoot, path, base string) ConfigRun {
	run := ConfigRun{Path: path}
	var err error
	if opts.Isolated {
		run.Result, err = checkIsolated(log, repoRoot, path, base)
	} else {
		var cfg config.Config
		if cfg, err = config.LoadConfig(path); err == nil {
			run.Result, err = groups(cfg, repoRoot, base, log, opts.Mode)
		}
	}
	if err != nil && len(run.Result.Groups) == 0 {
		run.Err = err
	}
	return run
}

func discoverConfigs(log commandLog, opts Options) (repoRoot string, paths []string, base string, err error) {
	repoRoot, err = gitRepoRoot(log, opts.RepoRoot)
	if err != nil {
		return "", nil, "", err
	}
	paths = opts.Paths
	if len(paths) == 0 {
		paths, err = discover.FindAll(opts.Context, repoRoot, opts.Isolated)
		if err != nil {
			return "", nil, "", err
		}
	}
	paths, err = normalizeConfigPaths(paths)
	if err != nil {
		return "", nil, "", err
	}
	var tree config.Tree
	if opts.Isolated {
		tree = discover.Committed(opts.Context, repoRoot)
	}
	if err := config.RejectOutputOverlaps(paths, tree); err != nil {
		return "", nil, "", err
	}
	base, err = sinceBase(log, repoRoot, opts.Since)
	if err != nil {
		return "", nil, "", err
	}
	return repoRoot, paths, base, nil
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
