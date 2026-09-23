package check

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ConfigRun is the outcome of loading and checking one config file.
// Err is a load or setup failure (invalid YAML, not a git work tree);
// groups did not run in that case.
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

// RunResult is the aggregated outcome of CheckAll.
// Configs is sorted by Path, regardless of execution strategy.
// RepoRoot is the git toplevel the run resolved, including when Paths
// named configs outside that tree.
type RunResult struct {
	RepoRoot string
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

// Counts returns the number of configs and the summed group statuses.
// Config-level setup failures are not included in the group totals;
// they still force ExitCode 2.
func (r RunResult) Counts() (configs, ok, drift, errors int) {
	configs = len(r.Configs)
	for _, cfg := range r.Configs {
		if cfg.Err != nil {
			continue
		}
		gOK, gDrift, gErr := cfg.Result.Counts()
		ok += gOK
		drift += gDrift
		errors += gErr
	}
	return configs, ok, drift, errors
}

// SummaryLines returns one block per config (header, then group summaries
// or a setup error), then a config-level totals line. Paths under RepoRoot
// are shown relative to it. Setup failures count as config errors in the
// totals; they are separate from the group totals inside each block.
func (r RunResult) SummaryLines() []string {
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
		if cfg.Result.cleanup != nil {
			lines = append(lines, fmt.Sprintf("  error (%s)", oneLineError(cfg.Result.cleanup)))
		}
	}
	if len(r.Configs) > 0 {
		lines = append(lines, "")
	}
	lines = append(lines, r.configStatusLine())
	return lines
}

// SuccessLines lists each config path, relative to RepoRoot, then the
// config-level totals line. Each config that skipped a group then adds a
// blank line, its path, and that config's group summary. Callers print
// this when ExitCode is 0.
func (r RunResult) SuccessLines() []string {
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

// FinalErrorLine is the aggregated error line for the run.
// A single failing or drifting config keeps that config's own detail.
// More than one is summarized by config counts. An empty string means
// every config passed.
func (r RunResult) FinalErrorLine() string {
	var failed, drifted []ConfigRun
	for _, cfg := range r.Configs {
		switch cfg.ExitCode() {
		case 2:
			failed = append(failed, cfg)
		case 1:
			drifted = append(drifted, cfg)
		}
	}
	switch {
	case len(failed) > 0 && len(drifted) > 0:
		return fmt.Sprintf(
			"error: %s failed; %s drifted",
			countNoun(len(failed), "config", "configs"),
			countNoun(len(drifted), "config", "configs"),
		)
	case len(failed) == 1:
		cfg := failed[0]
		if cfg.Err != nil {
			return "error: " + oneLineError(cfg.Err)
		}
		return cfg.Result.FinalErrorLine()
	case len(failed) > 1:
		return fmt.Sprintf("error: %d configs failed", len(failed))
	case len(drifted) == 1:
		return drifted[0].Result.FinalErrorLine()
	case len(drifted) > 1:
		n := 0
		for _, cfg := range drifted {
			n += len(cfg.Result.AllDrifts())
		}
		return fmt.Sprintf(
			"error: %s drifted; commit the generator output or fix the command",
			countNoun(n, "generated path", "generated paths"),
		)
	default:
		return ""
	}
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
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}
