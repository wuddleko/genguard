package check

import (
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
			run.Configs = append(run.Configs, checkOneIsolated(configPath, base))
		}
		return run, nil
	}
	damage := map[string]pathSnap{}
	for _, configPath := range paths {
		run.Configs = append(run.Configs, checkOne(configPath, base, damage))
	}
	return run, nil
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

func checkOneIsolated(path, since string) ConfigRun {
	result, err := CheckSinceIsolated(path, since)
	return isolatedRun(path, result, err)
}

func isolatedRun(configPath string, result ConfigResult, err error) ConfigRun {
	if err != nil && len(result.Groups) > 0 {
		result.noteCleanup(err)
	}
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
		detail := strings.TrimSpace(out)
		if detail == "" {
			detail = "git ls-tree failed"
		}
		return nil, newGenguardError("%s", detail)
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

func checkOne(path, base string, damage map[string]pathSnap) ConfigRun {
	cfgRun := ConfigRun{Path: path}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		cfgRun.Err = err
		return cfgRun
	}
	result, err := checkConfig(cfg, base, damage)
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
