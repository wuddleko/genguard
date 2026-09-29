package check

import (
	"context"
	"io"
	"path/filepath"
	"time"

	"github.com/wuddleko/genguard/internal/actions"
	"github.com/wuddleko/genguard/internal/config"
)

// Tests build config.Config values with no file behind them, so they run the
// groups directly instead of through Execute.

func checkConfig(cfg config.Config, base string, log commandLog) (ConfigResult, error) {
	return groupsInRepo(cfg, base, log, ModeCheck)
}

func runConfig(cfg config.Config, base string, log commandLog) (ConfigResult, error) {
	return groupsInRepo(cfg, base, log, ModeRun)
}

func groupsInRepo(cfg config.Config, base string, log commandLog, mode Mode) (ConfigResult, error) {
	repoRoot, err := gitRepoRoot(log, cfg.Root())
	if err != nil {
		return ConfigResult{}, err
	}
	return groups(cfg, repoRoot, base, log, mode)
}

func runCommand(root, commandText string, log io.Writer, header string, timeout time.Duration) (string, error) {
	return runCommandContext(context.Background(), root, commandText, log, header, timeout, actions.Env{})
}

// singleReport is the report of one config at repoRoot/genguard.yaml.
func singleReport(result ConfigResult, repoRoot string) (string, error) {
	return FormatFailureReport(RunResult{
		RepoRoot: repoRoot,
		Configs:  []ConfigRun{{Path: filepath.Join(repoRoot, "genguard.yaml"), Result: result}},
	})
}

// groupDrift is the drift of group for a config in dir.
func groupDrift(dir string, group config.Group) ([]Drift, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	repoRoot, err := gitRepoRoot(commandLog{}, abs)
	if err != nil {
		return nil, err
	}
	t, err := newTree(repoRoot, abs)
	if err != nil {
		return nil, err
	}
	return driftForGroup(commandLog{}, t, group)
}
