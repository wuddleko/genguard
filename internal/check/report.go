package check

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FormatFailureReport renders the human-readable failure output: summary,
// drift lines, optional git diff, and the final error line.
//
// If the git diff cannot be produced, the summary and drift lines are still
// returned (without the final error line) along with the error.
func FormatFailureReport(result ConfigResult, root string) (string, error) {
	var b strings.Builder
	b.WriteString("Summary\n")
	for _, line := range result.SummaryLines() {
		b.WriteString(line)
		b.WriteString("\n")
	}

	drifts := result.AllDrifts()
	if len(drifts) > 0 {
		b.WriteString("\nDrift\n")
		for _, item := range drifts {
			fmt.Fprintf(&b, "[%s] %s: %s\n", item.Kind, item.Group, item.Path)
		}
		diff, err := DriftDiff(root, drifts)
		if err != nil {
			return b.String(), err
		}
		if strings.TrimSpace(diff) != "" {
			b.WriteString("\n")
			b.WriteString(diff)
			b.WriteString("\n")
		}
	}

	if line := result.FinalErrorLine(); line != "" {
		b.WriteString("\n")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// FormatRunFailureReport renders a multi-config failure: a summary with
// one block per config, drift lines and git diffs for configs that
// drifted, and an aggregated final error line.
//
// Diffs are taken from each config's directory. If a git diff cannot be
// produced, the report built so far is returned without the final error
// line, along with the error.
func FormatRunFailureReport(run RunResult) (string, error) {
	var b strings.Builder
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
		b.WriteString(displayConfigPath(run.RepoRoot, cfg.Path))
		b.WriteString("\n")
		for _, item := range drifts {
			fmt.Fprintf(&b, "[%s] %s: %s\n", item.Kind, item.Group, item.Path)
		}
		diff, err := DriftDiff(filepath.Dir(cfg.Path), drifts)
		if err != nil {
			return b.String(), err
		}
		if strings.TrimSpace(diff) != "" {
			b.WriteString("\n")
			b.WriteString(diff)
			b.WriteString("\n")
		}
	}

	if line := run.FinalErrorLine(); line != "" {
		b.WriteString("\n")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), nil
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
