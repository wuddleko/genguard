package check

import (
	"github.com/wuddleko/genguard/internal/config"
)

func RunConfig(cfg config.Config) (ConfigResult, error) {
	return runConfig(cfg, "")
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
		run.Configs = append(run.Configs, runOne(configPath, base))
	}
	return run, nil
}

func runOne(path, base string) ConfigRun {
	cfgRun := ConfigRun{Path: path}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		cfgRun.Err = err
		return cfgRun
	}
	result, err := runConfig(cfg, base)
	if err != nil {
		cfgRun.Err = err
		return cfgRun
	}
	cfgRun.Result = result
	return cfgRun
}

func RunSince(cfg config.Config, since string) (ConfigResult, error) {
	base, err := sinceBase(cfg, since)
	if err != nil {
		return ConfigResult{}, err
	}
	if base == "" {
		return RunConfig(cfg)
	}
	return runGroups(cfg, base)
}

func runConfig(cfg config.Config, base string) (ConfigResult, error) {
	if err := requireGitRepo(cfg.Root()); err != nil {
		return ConfigResult{}, err
	}
	return runGroups(cfg, base)
}

func runGroups(cfg config.Config, base string) (ConfigResult, error) {
	root := cfg.Root()
	result := ConfigResult{}
	for _, group := range cfg.Groups {
		result.Groups = append(result.Groups, runGroup(root, group, base, cfg.Path))
	}
	return result, nil
}

func runGroup(root string, group config.Group, base, configPath string) GroupResult {
	return runPreparedGroup(root, group, base, configPath, nil, nil)
}
