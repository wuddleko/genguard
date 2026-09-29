package check

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/wuddleko/genguard/internal/command"
	"github.com/wuddleko/genguard/internal/config"
)

var versionPattern = regexp.MustCompile(`v?\d+\.\d+(?:\.\d+)?(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?`)

type toolKey struct {
	command string
	want    string
	timeout time.Duration
}

type cachedProbe struct {
	have   string
	detail string
}

// toolCache holds the probes of one config. Probes run in the config
// directory, so a cache is never shared across configs.
type toolCache map[toolKey]cachedProbe

func (p cachedProbe) apply(name, want string) (ToolResult, error) {
	item := ToolResult{Name: name, Want: want, Have: p.have}
	if p.detail == "" {
		return item, nil
	}
	return item, fmt.Errorf("%s: %s", name, p.detail)
}

func verifyTools(root string, declared []config.Tool, names []string, timeout time.Duration, log commandLog, probes toolCache) ([]ToolResult, error) {
	byName := make(map[string]config.Tool, len(declared))
	for _, tool := range declared {
		byName[tool.Name] = tool
	}
	observed := make([]ToolResult, 0, len(names))
	var first error
	for _, name := range names {
		tool, ok := byName[name]
		if !ok {
			observed = append(observed, ToolResult{Name: name})
			if first == nil {
				first = fmt.Errorf("%s: unknown tool", name)
			}
			continue
		}
		item, err := probeTool(root, tool, timeout, log, probes)
		observed = append(observed, item)
		if err != nil && first == nil {
			first = err
		}
	}
	return observed, first
}

func probeTool(root string, tool config.Tool, timeout time.Duration, log commandLog, probes toolCache) (ToolResult, error) {
	item := ToolResult{Name: tool.Name, Want: pinVersion(tool.Version)}
	commandText := strings.TrimSpace(tool.Command)
	if commandText == "" {
		commandText = tool.Name + " --version"
	}
	key := toolKey{command: commandText, want: item.Want, timeout: timeout}
	if hit, ok := probes[key]; ok {
		return hit.apply(tool.Name, item.Want)
	}
	item, detail, err := runProbe(root, tool, commandText, item, timeout, log)
	if isInterrupt(err) {
		return item, err
	}
	probes[key] = cachedProbe{have: item.Have, detail: detail}
	return item, err
}

func runProbe(root string, tool config.Tool, commandText string, item ToolResult, timeout time.Duration, log commandLog) (ToolResult, string, error) {
	fail := func(detail string) (ToolResult, string, error) {
		return item, detail, fmt.Errorf("%s: %s", tool.Name, detail)
	}
	if strings.TrimSpace(tool.Command) == "" {
		if _, err := exec.LookPath(tool.Name); err != nil {
			return fail("not on PATH")
		}
	}
	output, code, err := command.Capture(log.ctx, root, commandText, timeout)
	if err != nil {
		var start *command.StartError
		if errors.As(err, &start) {
			return fail("not on PATH")
		}
		if errors.Is(err, command.ErrInterrupted) {
			return item, "", err
		}
		var timedOut *command.TimeoutError
		if errors.As(err, &timedOut) {
			return fail(err.Error())
		}
		if code == 127 {
			return fail("not on PATH")
		}
		if code > 0 {
			return fail(fmt.Sprintf("version command failed (exit %d)", code))
		}
		return fail("version command failed")
	}
	have, ok := observedVersion(output)
	if !ok {
		return fail("version command returned no version")
	}
	item.Have = have
	if item.Want != "" && item.Want != have {
		return fail(fmt.Sprintf("want %s, have %s", item.Want, have))
	}
	return item, "", nil
}

func pinVersion(version string) string {
	return stripVersionPrefix(strings.TrimSpace(version))
}

func observedVersion(output string) (string, bool) {
	match := versionPattern.FindString(output)
	if match == "" {
		return "", false
	}
	return stripVersionPrefix(match), true
}

func stripVersionPrefix(version string) string {
	if len(version) >= 2 && (version[0] == 'v' || version[0] == 'V') && version[1] >= '0' && version[1] <= '9' {
		return version[1:]
	}
	return version
}
