package check

import (
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/wuddleko/genguard/internal/check/command"
	"github.com/wuddleko/genguard/internal/config"
)

// versionPattern is the first semver-shaped token in a version command's output.
// A leading v is optional. One dotted pair is required, then an optional third
// component, pre-release, and build.
var versionPattern = regexp.MustCompile(`v?\d+\.\d+(?:\.\d+)?(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?`)

func verifyTools(root string, declared []config.Tool, names []string, timeout time.Duration) ([]ToolResult, error) {
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
				first = newGenguardError("%s: unknown tool", name)
			}
			continue
		}
		item, err := probeTool(root, tool, timeout)
		observed = append(observed, item)
		if err != nil && first == nil {
			first = err
		}
	}
	return observed, first
}

func probeTool(root string, tool config.Tool, timeout time.Duration) (ToolResult, error) {
	item := ToolResult{Name: tool.Name, Want: pinVersion(tool.Version)}
	commandText := strings.TrimSpace(tool.Command)
	if commandText == "" {
		if _, err := exec.LookPath(tool.Name); err != nil {
			return item, newGenguardError("%s: not on PATH", tool.Name)
		}
		commandText = tool.Name + " --version"
	}
	output, code, err := command.Capture(root, commandText, timeout)
	if err != nil {
		if strings.HasPrefix(err.Error(), "command timed out after ") {
			return item, newGenguardError("%s: %s", tool.Name, err.Error())
		}
		if code == 127 {
			return item, newGenguardError("%s: not on PATH", tool.Name)
		}
		if code > 0 {
			return item, newGenguardError("%s: version command failed (exit %d)", tool.Name, code)
		}
		return item, newGenguardError("%s: version command failed", tool.Name)
	}
	have, ok := observedVersion(output)
	if !ok {
		return item, newGenguardError("%s: version command returned no version", tool.Name)
	}
	item.Have = have
	if item.Want != "" && item.Want != have {
		return item, newGenguardError("%s: want %s, have %s", tool.Name, item.Want, have)
	}
	return item, nil
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
