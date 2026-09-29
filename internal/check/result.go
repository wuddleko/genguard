package check

import (
	"fmt"
	"strings"
)

type GroupStatus string

const (
	GroupOK      GroupStatus = "ok"
	GroupDrift   GroupStatus = "drift"
	GroupError   GroupStatus = "error"
	GroupSkipped GroupStatus = "skipped"
)

type ToolResult struct {
	Name string
	Want string
	Have string
}

type GroupResult struct {
	Name        string
	Status      GroupStatus
	Drifts      []Drift
	Err         error
	CommandTail string
	Tools       []ToolResult
}

type ConfigResult struct {
	Groups   []GroupResult
	captured *capturedDriftDiff
	// extra is an error outside any group: the run was interrupted, or the
	// isolated worktree could not be removed.
	extra error
}

type capturedDriftDiff struct {
	text string
	err  error
}

func (r *ConfigResult) captureDriftDiff(text string, err error) {
	r.captured = &capturedDriftDiff{text: text, err: err}
}

// note keeps the first error, except that any other error replaces an
// interrupt.
func (r *ConfigResult) note(err error) {
	if err == nil || (r.extra != nil && !isInterrupt(r.extra)) {
		return
	}
	r.extra = err
}

func (r ConfigResult) hasInterrupt() bool {
	if isInterrupt(r.extra) {
		return true
	}
	for _, group := range r.Groups {
		if isInterrupt(group.Err) {
			return true
		}
	}
	return false
}

func (r ConfigResult) AllDrifts() []Drift {
	all := make([]Drift, 0)
	for _, group := range r.Groups {
		all = append(all, group.Drifts...)
	}
	return all
}

func (r ConfigResult) ExitCode() int {
	if r.extra != nil {
		return 2
	}
	_, drift, errors := r.Counts()
	if errors > 0 {
		return 2
	}
	if drift > 0 {
		return 1
	}
	return 0
}

func (r ConfigResult) Counts() (ok, drift, errors int) {
	for _, group := range r.Groups {
		switch group.Status {
		case GroupOK:
			ok++
		case GroupDrift:
			drift++
		case GroupError:
			errors++
		case GroupSkipped:
		default:
			errors++
		}
	}
	return ok, drift, errors
}

func (g GroupResult) SummaryLine() string {
	switch g.Status {
	case GroupOK:
		return withToolVersions(fmt.Sprintf("  %s: OK", g.Name), g.Tools)
	case GroupDrift:
		line := fmt.Sprintf("  %s: drift (%s)", g.Name, driftKindSummary(g.Drifts))
		return withToolVersions(line, g.Tools)
	case GroupError:
		line := fmt.Sprintf("  %s: error (%s)", g.Name, oneLineError(g.Err))
		if len(g.Drifts) > 0 {
			line += "; drift (" + driftKindSummary(g.Drifts) + ")"
		}
		return withToolVersions(line, matchedTools(g.Tools))
	case GroupSkipped:
		return fmt.Sprintf("  %s: skipped", g.Name)
	default:
		return fmt.Sprintf("  %s: unknown", g.Name)
	}
}

func matchedTools(tools []ToolResult) []ToolResult {
	matched := make([]ToolResult, 0, len(tools))
	for _, tool := range tools {
		if tool.Have == "" || (tool.Want != "" && tool.Want != tool.Have) {
			continue
		}
		matched = append(matched, tool)
	}
	return matched
}

func withToolVersions(line string, tools []ToolResult) string {
	parts := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.Have == "" {
			continue
		}
		parts = append(parts, tool.Name+" "+tool.Have)
	}
	if len(parts) == 0 {
		return line
	}
	return line + "; " + strings.Join(parts, ", ")
}

func (r ConfigResult) SummaryLines() []string {
	lines := make([]string, 0, len(r.Groups)+1)
	for _, group := range r.Groups {
		lines = append(lines, group.SummaryLine())
	}
	ok, drift, errors := r.Counts()
	line := fmt.Sprintf(
		"%s: %d ok, %d drift, %d error",
		countNoun(len(r.Groups), "group", "groups"),
		ok,
		drift,
		errors,
	)
	if n := r.Skipped(); n > 0 {
		line += ", " + countNoun(n, "skipped", "skipped")
	}
	lines = append(lines, line)
	return lines
}

func (r ConfigResult) Skipped() int {
	n := 0
	for _, group := range r.Groups {
		if group.Status == GroupSkipped {
			n++
		}
	}
	return n
}

func (r ConfigResult) FinalErrorLine() string {
	_, drift, errors := r.Counts()
	oneFailed := "error: 1 group failed"
	if err := r.firstGroupError(); err != nil {
		oneFailed = "error: " + oneLineError(err)
	}
	line := failureLine(errors, drift, "group", oneFailed, len(r.AllDrifts()))
	if r.extra == nil {
		return line
	}
	extra := "error: " + oneLineError(r.extra)
	if line == "" {
		return extra
	}
	return line + "\n" + extra
}

// failureLine is the last line of a failed run of groups or configs.
// oneFailed is the line when exactly one of them failed.
func failureLine(failed, drifted int, unit, oneFailed string, driftedPaths int) string {
	units := unit + "s"
	switch {
	case failed > 0 && drifted > 0:
		return fmt.Sprintf("error: %s failed; %s drifted", countNoun(failed, unit, units), countNoun(drifted, unit, units))
	case failed == 1:
		return oneFailed
	case failed > 1:
		return fmt.Sprintf("error: %d %s failed", failed, units)
	case drifted > 0:
		return fmt.Sprintf(
			"error: %s drifted; commit the generator output or fix the command",
			countNoun(driftedPaths, "generated path", "generated paths"),
		)
	default:
		return ""
	}
}

func (r ConfigResult) firstGroupError() error {
	for _, group := range r.Groups {
		if group.Status == GroupError {
			return group.Err
		}
	}
	return nil
}

func oneLineError(err error) string {
	if err == nil {
		return "unknown error"
	}
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if msg == "" {
		return "unknown error"
	}
	return msg
}

func countNoun(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}
