package check

import (
	"fmt"
	"strings"
)

func FormatFailureReport(run RunResult) (string, error) {
	var b strings.Builder
	b.WriteString(FormatCommandTails(run))
	b.WriteString("Summary\n")
	for _, line := range run.SummaryLines() {
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
		if run.All {
			b.WriteString(displayConfigPath(run.RepoRoot, cfg.Path))
			b.WriteString("\n")
		}
		if err := writeDriftBody(&b, run.RepoRoot, cfg.Result, drifts); err != nil {
			return b.String(), err
		}
	}

	if line := run.FinalErrorLine(); line != "" {
		b.WriteString("\n")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// FormatCommandTails is the output of each failed command, labeled with the
// group name, or under --all with the config path and the group name.
func FormatCommandTails(run RunResult) string {
	var b strings.Builder
	for _, cfg := range run.Configs {
		if cfg.Err != nil {
			continue
		}
		prefix := ""
		if run.All {
			prefix = displayConfigPath(run.RepoRoot, cfg.Path) + ": "
		}
		for _, group := range cfg.Result.Groups {
			tail := group.CommandTail
			if tail == "" {
				continue
			}
			b.WriteString(prefix + group.Name + ":\n")
			b.WriteString(tail)
			if !strings.HasSuffix(tail, "\n") {
				b.WriteByte('\n')
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func writeDriftBody(b *strings.Builder, repoRoot string, result ConfigResult, drifts []Drift) error {
	for _, item := range drifts {
		fmt.Fprintf(b, "[%s] %s: %s\n", item.Kind, item.Group, item.Path)
	}
	diff, err := resultDriftDiff(repoRoot, result, drifts)
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

func resultDriftDiff(repoRoot string, result ConfigResult, drifts []Drift) (string, error) {
	if result.captured != nil {
		return result.captured.text, result.captured.err
	}
	return driftDiff(repoRoot, drifts)
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
