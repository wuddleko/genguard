package check

import (
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
	for _, configPath := range paths {
		run.Configs = append(run.Configs, runOne(configPath, base, streamFor(repoRoot, configPath, opts.Log, opts.Quiet)))
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

func RunSinceLog(cfg config.Config, since string, log io.Writer, quiet bool) (ConfigResult, error) {
	return runSince(cfg, since, commandLog{w: log, quiet: quiet})
}

func runSince(cfg config.Config, since string, log commandLog) (ConfigResult, error) {
	base, err := sinceBase(cfg, since)
	if err != nil {
		return ConfigResult{}, err
	}
	if base == "" {
		return runConfig(cfg, "", log)
	}
	return runGroups(cfg, base, log)
}

func runConfig(cfg config.Config, base string, log commandLog) (ConfigResult, error) {
	if err := requireGitRepo(cfg.Root()); err != nil {
		return ConfigResult{}, err
	}
	return runGroups(cfg, base, log)
}

func runGroups(cfg config.Config, base string, log commandLog) (ConfigResult, error) {
	root := cfg.Root()
	result := ConfigResult{}
	for _, group := range cfg.Groups {
		result.Groups = append(result.Groups, runPreparedGroup(root, group, base, cfg.Path, log, nil, nil))
	}
	return result, nil
}
