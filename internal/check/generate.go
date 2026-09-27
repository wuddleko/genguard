package check

import (
	"context"
	"io"

	"github.com/wuddleko/genguard/internal/config"
)

func RunConfig(cfg config.Config) (ConfigResult, error) {
	return runConfig(cfg, "", commandLog{})
}

func RunAll(opts CheckAllOptions) (RunResult, error) {
	if opts.Isolated {
		return RunResult{}, newGenguardError("genguard run writes the checkout; --isolated is not valid")
	}
	repoRoot, paths, base, err := discoverConfigs(opts)
	if err != nil {
		return RunResult{}, err
	}
	run := RunResult{
		RepoRoot: repoRoot,
		Configs:  make([]ConfigRun, 0, len(paths)),
	}
	cache := &toolCache{}
	for _, configPath := range paths {
		if stop, err := canceledStop(opts.Context, run.ExitCode(), func(interrupt error) { noteInterruptedDrift(&run, interrupt) }); stop {
			return run, err
		}
		log := streamFor(repoRoot, configPath, opts.Log, opts.Quiet)
		log.ctx = opts.Context
		log.toolCache = cache
		run.Configs = append(run.Configs, runOne(configPath, base, log))
	}
	return run, nil
}

func runOne(path, base string, log commandLog) ConfigRun {
	return loadConfigRun(path, func(cfg config.Config) (ConfigResult, error) {
		return runConfig(cfg, base, log)
	})
}

func RunSince(cfg config.Config, since string) (ConfigResult, error) {
	return runSince(cfg, since, commandLog{})
}

func RunSinceLog(ctx context.Context, cfg config.Config, since string, log io.Writer, quiet bool) (ConfigResult, error) {
	return runSince(cfg, since, commandLog{w: log, quiet: quiet, ctx: ctx})
}

func runSince(cfg config.Config, since string, log commandLog) (ConfigResult, error) {
	base, err := sinceBase(log, cfg, since)
	if err != nil {
		return ConfigResult{}, err
	}
	if base == "" {
		return runConfig(cfg, "", log)
	}
	return runGroups(cfg, base, log)
}

func runConfig(cfg config.Config, base string, log commandLog) (ConfigResult, error) {
	if err := requireGitRepo(log, cfg.Root()); err != nil {
		return ConfigResult{}, err
	}
	return runGroups(cfg, base, log)
}

func runGroups(cfg config.Config, base string, log commandLog) (ConfigResult, error) {
	log = log.withToolCache()
	root := cfg.Root()
	result := ConfigResult{}
	for _, group := range cfg.Groups {
		if stop, err := canceledStop(log.ctx, result.ExitCode(), result.noteCleanup); stop {
			return result, err
		}
		result.Groups = append(result.Groups, runPreparedGroup(root, group, cfg.Tools, base, cfg.Path, log, nil, nil))
	}
	if stop, err := canceledStop(log.ctx, result.ExitCode(), result.noteCleanup); stop {
		return result, err
	}
	return result, nil
}
