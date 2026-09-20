package check

import (
	"fmt"
	"strings"
)

type GroupStatus string

const (
	GroupOK    GroupStatus = "ok"
	GroupDrift GroupStatus = "drift"
	GroupError GroupStatus = "error"
)

type GroupResult struct {
	Name   string
	Status GroupStatus
	Drifts []Drift
	Err    error
}

type ConfigResult struct {
	Groups []GroupResult
}

func (r ConfigResult) AllDrifts() []Drift {
	all := make([]Drift, 0)
	for _, group := range r.Groups {
		all = append(all, group.Drifts...)
	}
	return all
}

func (r ConfigResult) ExitCode() int {
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
		default:
			errors++
		}
	}
	return ok, drift, errors
}

func (g GroupResult) SummaryLine() string {
	switch g.Status {
	case GroupOK:
		return fmt.Sprintf("  %s: OK", g.Name)
	case GroupDrift:
		n := len(g.Drifts)
		if n == 1 {
			return fmt.Sprintf("  %s: drift (1 file)", g.Name)
		}
		return fmt.Sprintf("  %s: drift (%d files)", g.Name, n)
	case GroupError:
		return fmt.Sprintf("  %s: error (%s)", g.Name, oneLineError(g.Err))
	default:
		return fmt.Sprintf("  %s: unknown", g.Name)
	}
}

func (r ConfigResult) SummaryLines() []string {
	lines := make([]string, 0, len(r.Groups)+1)
	for _, group := range r.Groups {
		lines = append(lines, group.SummaryLine())
	}
	ok, drift, errors := r.Counts()
	lines = append(lines, fmt.Sprintf(
		"%s: %d ok, %d drift, %d error",
		countNoun(len(r.Groups), "group", "groups"),
		ok,
		drift,
		errors,
	))
	return lines
}

func (r ConfigResult) FinalErrorLine() string {
	_, drift, errors := r.Counts()
	switch {
	case errors > 0 && drift > 0:
		return fmt.Sprintf("error: %s failed; %s drifted", countNoun(errors, "group", "groups"), countNoun(drift, "group", "groups"))
	case errors > 0:
		if errors == 1 {
			if err := r.firstGroupError(); err != nil {
				return "error: " + err.Error()
			}
			return "error: 1 group failed"
		}
		return fmt.Sprintf("error: %d groups failed", errors)
	case drift > 0:
		return fmt.Sprintf(
			"error: %s drifted; commit the generator output or fix the command",
			countNoun(len(r.AllDrifts()), "generated path", "generated paths"),
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
