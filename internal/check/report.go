package check

import (
	"fmt"
	"path/filepath"
	"strings"
)

func FormatFailureReport(result ConfigResult, root string) (string, error) {
	return renderFailureReport(RunResult{
		Configs: []ConfigRun{{
			Path:   filepath.Join(root, "genguard.yaml"),
			Result: result,
		}},
	}, true)
}

func FormatRunFailureReport(run RunResult) (string, error) {
	return renderFailureReport(run, false)
}

func renderFailureReport(run RunResult, singleConfig bool) (string, error) {
	var b strings.Builder
	if singleConfig {
		b.WriteString(singleCommandTails(run))
	} else {
		b.WriteString(FormatRunCommandTails(run))
	}
	b.WriteString("Summary\n")
	for _, line := range failureSummaryLines(run, singleConfig) {
		b.WriteString(line)
		b.WriteString("\n")
	}

	wroteDrift := false
	for _, cfg := range run.Configs {
		if cfg.Err != nil {
			continue
		}
		drifts := cfg.Result.AllDrifts()
		if len(drifts) == 0 {
			continue
		}
		if !wroteDrift {
			b.WriteString("\nDrift\n")
			wroteDrift = true
		} else {
			b.WriteString("\n")
		}
		repoRoot := run.RepoRoot
		configDir := filepath.Dir(cfg.Path)
		if singleConfig {
			repoRoot = ""
		} else {
			b.WriteString(displayConfigPath(run.RepoRoot, cfg.Path))
			b.WriteString("\n")
		}
		if err := writeDriftBody(&b, cfg.Result, newDriftBase(configDir, repoRoot), drifts); err != nil {
			return b.String(), err
		}
	}

	line := run.FinalErrorLine()
	if singleConfig {
		line = singleFinalErrorLine(run)
	}
	if line != "" {
		b.WriteString("\n")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), nil
}

func singleCommandTails(run RunResult) string {
	if len(run.Configs) != 1 || run.Configs[0].Err != nil {
		return ""
	}
	return FormatCommandTails(run.Configs[0].Result)
}

func failureSummaryLines(run RunResult, singleConfig bool) []string {
	if !singleConfig {
		return run.SummaryLines()
	}
	if len(run.Configs) != 1 || run.Configs[0].Err != nil {
		return nil
	}
	return run.Configs[0].Result.SummaryLines()
}

func singleFinalErrorLine(run RunResult) string {
	if len(run.Configs) != 1 || run.Configs[0].Err != nil {
		return run.FinalErrorLine()
	}
	return run.Configs[0].Result.FinalErrorLine()
}

func FormatCommandTails(result ConfigResult) string {
	var b strings.Builder
	writeCommandTails(&b, result.Groups, func(name string) string {
		return name + ":"
	})
	return b.String()
}

func FormatRunCommandTails(run RunResult) string {
	var b strings.Builder
	for _, cfg := range run.Configs {
		if cfg.Err != nil {
			continue
		}
		path := displayConfigPath(run.RepoRoot, cfg.Path)
		writeCommandTails(&b, cfg.Result.Groups, func(name string) string {
			return path + ": " + name + ":"
		})
	}
	return b.String()
}

func writeCommandTails(b *strings.Builder, groups []GroupResult, label func(name string) string) {
	for _, group := range groups {
		tail := group.CommandTail
		if tail == "" {
			continue
		}
		b.WriteString(label(group.Name))
		b.WriteByte('\n')
		b.WriteString(tail)
		if !strings.HasSuffix(tail, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
}

func writeDriftBody(b *strings.Builder, result ConfigResult, base driftBase, drifts []Drift) error {
	for _, item := range drifts {
		fmt.Fprintf(b, "[%s] %s: %s\n", item.Kind, item.Group, base.shown(item.Path))
	}
	diff, err := resultDriftDiff(result, base, drifts)
	if err != nil {
		return err
	}
	if strings.TrimSpace(diff) != "" {
		b.WriteString("\n")
		b.WriteString(diff)
		b.WriteString("\n")
	}
	return nil
}

func resultDriftDiff(result ConfigResult, base driftBase, drifts []Drift) (string, error) {
	if result.captured != nil {
		return result.captured.text, result.captured.err
	}
	return driftDiff(base, drifts)
}

func driftKindSummary(drifts []Drift) string {
	if len(drifts) == 0 {
		return "0 files"
	}
	counts := make(map[string]int, len(drifts))
	extra := make([]string, 0)
	for _, drift := range drifts {
		kind := drift.Kind
		if counts[kind] == 0 && kind != "modified" && kind != "untracked" && kind != "missing" {
			extra = append(extra, kind)
		}
		counts[kind]++
	}
	parts := make([]string, 0, 3+len(extra))
	for _, kind := range []string{"modified", "untracked", "missing"} {
		if n := counts[kind]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, kind))
		}
	}
	for _, kind := range extra {
		n := counts[kind]
		if kind == "" {
			parts = append(parts, countNoun(n, "file", "files"))
			continue
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, kind))
	}
	return strings.Join(parts, ", ")
}
