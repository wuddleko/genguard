package check

import (
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

type CheckAllOptions struct {
	RepoRoot string
	Paths    []string
	Since    string
	Isolated bool
	Log      io.Writer
}

func CheckAll(opts CheckAllOptions) (RunResult, error) {
	repoRoot, paths, base, err := discoverConfigs(opts)
	if err != nil {
		return RunResult{}, err
	}
	run := RunResult{
		RepoRoot: repoRoot,
		Configs:  make([]ConfigRun, 0, len(paths)),
	}
	if opts.Isolated {
		for _, configPath := range paths {
			run.Configs = append(run.Configs, checkOneIsolated(configPath, base, streamFor(repoRoot, configPath, opts.Log)))
		}
		return run, nil
	}
	damage := map[string]pathSnap{}
	for _, configPath := range paths {
		run.Configs = append(run.Configs, checkOne(configPath, base, damage, streamFor(repoRoot, configPath, opts.Log)))
	}
	return run, nil
}

func streamFor(repoRoot, configPath string, log io.Writer) commandLog {
	if log == nil {
		return commandLog{}
	}
	return commandLog{w: log, prefix: displayConfigPath(repoRoot, configPath) + ": "}
}

func discoverConfigs(opts CheckAllOptions) (repoRoot string, paths []string, base string, err error) {
	repoRoot, err = gitRepoRoot(opts.RepoRoot)
	if err != nil {
		return "", nil, "", err
	}

	paths = opts.Paths
	if len(paths) == 0 {
		var found []string
		if opts.Isolated {
			found, err = findCommittedConfigs(repoRoot)
		} else {
			found, err = config.FindAll(repoRoot)
		}
		if err != nil {
			return "", nil, "", err
		}
		paths = found
	}

	paths, err = normalizeConfigPaths(paths)
	if err != nil {
		return "", nil, "", err
	}
	if strings.TrimSpace(opts.Since) != "" {
		base, err = mergeBase(repoRoot, opts.Since)
		if err != nil {
			return "", nil, "", err
		}
	}
	return repoRoot, paths, base, nil
}

func checkOneIsolated(path, since string, log commandLog) ConfigRun {
	result, err := checkSinceIsolated(path, since, log)
	return isolatedRun(path, result, err)
}

func isolatedRun(configPath string, result ConfigResult, err error) ConfigRun {
	run := ConfigRun{Path: configPath, Result: result}
	if err != nil && len(result.Groups) == 0 {
		run.Err = err
	}
	return run
}

func findCommittedConfigs(repoRoot string) ([]string, error) {
	out, code, err := git(repoRoot, "ls-tree", "-r", "-z", "--name-only", "HEAD")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, newGenguardError("%s", gitDetail(out, "git ls-tree failed"))
	}
	found := make([]string, 0)
	for _, name := range parseGitNameList(out) {
		if !committedConfig(name) {
			continue
		}
		found = append(found, filepath.Join(repoRoot, filepath.FromSlash(name)))
	}
	sort.Strings(found)
	if err := config.RejectBothConfigNames(found); err != nil {
		return nil, err
	}
	return found, nil
}

func committedConfig(rel string) bool {
	if !config.IsConfigName(path.Base(rel)) {
		return false
	}
	dir := path.Dir(rel)
	if dir == "." {
		return true
	}
	for _, part := range strings.Split(dir, "/") {
		if config.SkipDir(part) {
			return false
		}
	}
	return true
}

func checkOne(path, base string, damage map[string]pathSnap, log commandLog) ConfigRun {
	return loadConfigRun(path, func(cfg config.Config) (ConfigResult, error) {
		return checkConfig(cfg, base, damage, log)
	})
}

func loadConfigRun(path string, run func(config.Config) (ConfigResult, error)) ConfigRun {
	cfgRun := ConfigRun{Path: path}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		cfgRun.Err = err
		return cfgRun
	}
	result, err := run(cfg)
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
