package check

import (
	"fmt"

	"github.com/wuddleko/genguard/internal/pathx"
)

type ConfigRun struct {
	Path   string
	Result ConfigResult
	Err    error
}

func (c ConfigRun) ExitCode() int {
	if c.Err != nil {
		return 2
	}
	return c.Result.ExitCode()
}

func (c ConfigRun) hasInterrupt() bool {
	return isInterrupt(c.Err) || c.Result.hasInterrupt()
}

func (c ConfigRun) finalErrorLine() string {
	if c.Err != nil {
		return "error: " + oneLineError(c.Err)
	}
	return c.Result.FinalErrorLine()
}

// RunResult is one Execute or ExecuteAll. All selects the --all report:
// config paths as labels and one summary per config.
type RunResult struct {
	RepoRoot string
	All      bool
	Configs  []ConfigRun
}

func (r RunResult) ExitCode() int {
	code := 0
	for _, cfg := range r.Configs {
		if n := cfg.ExitCode(); n > code {
			code = n
		}
		if code == 2 {
			return 2
		}
	}
	return code
}

func (r RunResult) Counts() (configs, ok, drift, errors int) {
	configs = len(r.Configs)
	for _, cfg := range r.Configs {
		if cfg.Err != nil {
			errors++
			continue
		}
		gOK, gDrift, gErr := cfg.Result.Counts()
		ok += gOK
		drift += gDrift
		errors += gErr
	}
	return configs, ok, drift, errors
}

func (r RunResult) single() (ConfigRun, bool) {
	if r.All || len(r.Configs) != 1 {
		return ConfigRun{}, false
	}
	return r.Configs[0], true
}

func (r RunResult) SummaryLines() []string {
	if cfg, ok := r.single(); ok {
		return cfg.Result.SummaryLines()
	}
	lines := make([]string, 0, len(r.Configs)*4+1)
	for i, cfg := range r.Configs {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, displayConfigPath(r.RepoRoot, cfg.Path))
		if cfg.Err != nil {
			lines = append(lines, fmt.Sprintf("  error (%s)", oneLineError(cfg.Err)))
			continue
		}
		lines = append(lines, cfg.Result.SummaryLines()...)
		if cfg.Result.extra != nil {
			lines = append(lines, fmt.Sprintf("  error (%s)", oneLineError(cfg.Result.extra)))
		}
	}
	if len(r.Configs) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, r.configStatusLine())
	return lines
}

// SuccessLines follow the success message: every config path under --all,
// and the summary of a config that skipped a group.
func (r RunResult) SuccessLines() []string {
	if cfg, ok := r.single(); ok {
		if cfg.Result.Skipped() == 0 {
			return nil
		}
		return cfg.Result.SummaryLines()
	}
	lines := make([]string, 0, len(r.Configs)+1)
	for _, cfg := range r.Configs {
		lines = append(lines, displayConfigPath(r.RepoRoot, cfg.Path))
	}
	lines = append(lines, r.configStatusLine())
	for _, cfg := range r.Configs {
		if cfg.Result.Skipped() == 0 {
			continue
		}
		lines = append(lines, "")
		lines = append(lines, displayConfigPath(r.RepoRoot, cfg.Path))
		lines = append(lines, cfg.Result.SummaryLines()...)
	}
	return lines
}

func (r RunResult) configStatusLine() string {
	ok, drift, errors := r.configStatusCounts()
	return fmt.Sprintf(
		"%s: %d ok, %d drift, %d error",
		countNoun(len(r.Configs), "config", "configs"),
		ok,
		drift,
		errors,
	)
}

func (r RunResult) FinalErrorLine() string {
	if cfg, ok := r.single(); ok {
		return cfg.finalErrorLine()
	}
	var failed, drifted []ConfigRun
	paths := 0
	for _, cfg := range r.Configs {
		switch cfg.ExitCode() {
		case 2:
			failed = append(failed, cfg)
		case 1:
			drifted = append(drifted, cfg)
			paths += len(cfg.Result.AllDrifts())
		}
	}
	oneFailed := ""
	if len(failed) == 1 {
		oneFailed = failed[0].finalErrorLine()
	}
	return failureLine(len(failed), len(drifted), "config", oneFailed, paths)
}

func (r RunResult) configStatusCounts() (ok, drift, errors int) {
	for _, cfg := range r.Configs {
		switch cfg.ExitCode() {
		case 2:
			errors++
		case 1:
			drift++
		default:
			ok++
		}
	}
	return ok, drift, errors
}

func displayConfigPath(repoRoot, path string) string {
	if repoRoot == "" || path == "" {
		return path
	}
	rel, ok := pathx.RelInside(repoRoot, path)
	if !ok {
		return path
	}
	return rel
}
