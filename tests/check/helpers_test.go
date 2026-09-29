package check_test

import (
	"path/filepath"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/config"
)

func checkConfig(cfg config.Config) (check.ConfigResult, error) {
	return execute(check.Options{}, cfg.Path)
}

func checkSince(cfg config.Config, since string) (check.ConfigResult, error) {
	return execute(check.Options{Since: since}, cfg.Path)
}

func runConfig(cfg config.Config) (check.ConfigResult, error) {
	return execute(check.Options{Mode: check.ModeRun}, cfg.Path)
}

func runSince(cfg config.Config, since string) (check.ConfigResult, error) {
	return execute(check.Options{Mode: check.ModeRun, Since: since}, cfg.Path)
}

func checkIsolated(path, since string) (check.ConfigResult, error) {
	return execute(check.Options{Isolated: true, Since: since}, path)
}

func execute(opts check.Options, path string) (check.ConfigResult, error) {
	run, err := check.Execute(opts, path)
	if err != nil {
		return check.ConfigResult{}, err
	}
	return run.Configs[0].Result, nil
}

// singleReport is the report of one config at repoRoot/genguard.yaml.
func singleReport(result check.ConfigResult, repoRoot string) (string, error) {
	return check.FormatFailureReport(check.RunResult{
		RepoRoot: repoRoot,
		Configs:  []check.ConfigRun{{Path: filepath.Join(repoRoot, "genguard.yaml"), Result: result}},
	})
}
