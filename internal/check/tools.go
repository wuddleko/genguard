package check

import (
	"fmt"
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

// toolKey identifies one probe. root is the working directory Capture uses,
// so the same command in another config or worktree is a different probe.
type toolKey struct {
	root    string
	command string
	want    string
}

type cachedProbe struct {
	have   string
	detail string
}

type toolCache struct {
	entries map[toolKey]cachedProbe
}

func (c *toolCache) lookup(root, command, want string) (cachedProbe, bool) {
	if c == nil || c.entries == nil {
		return cachedProbe{}, false
	}
	hit, ok := c.entries[toolKey{root: root, command: command, want: want}]
	return hit, ok
}

func (c *toolCache) remember(root, command, want, have, detail string) {
	if c == nil {
		return
	}
	if c.entries == nil {
		c.entries = map[toolKey]cachedProbe{}
	}
	c.entries[toolKey{root: root, command: command, want: want}] = cachedProbe{have: have, detail: detail}
}

func (p cachedProbe) apply(name, want string) (ToolResult, error) {
	item := ToolResult{Name: name, Want: want, Have: p.have}
	if p.detail == "" {
		return item, nil
	}
	return item, newGenguardError("%s: %s", name, p.detail)
}

func verifyTools(root string, declared []config.Tool, names []string, timeout time.Duration, cache *toolCache) ([]ToolResult, error) {
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
		item, err := probeTool(root, tool, timeout, cache)
		observed = append(observed, item)
		if err != nil && first == nil {
			first = err
		}
	}
	return observed, first
}

func probeTool(root string, tool config.Tool, timeout time.Duration, cache *toolCache) (ToolResult, error) {
	item := ToolResult{Name: tool.Name, Want: pinVersion(tool.Version)}
	commandText := strings.TrimSpace(tool.Command)
	if commandText == "" {
		commandText = tool.Name + " --version"
	}
	if hit, ok := cache.lookup(root, commandText, item.Want); ok {
		return hit.apply(tool.Name, item.Want)
	}
	item, detail, err := runProbe(root, tool, commandText, item, timeout)
	cache.remember(root, commandText, item.Want, item.Have, detail)
	return item, err
}

func runProbe(root string, tool config.Tool, commandText string, item ToolResult, timeout time.Duration) (ToolResult, string, error) {
	fail := func(detail string) (ToolResult, string, error) {
		return item, detail, newGenguardError("%s: %s", tool.Name, detail)
	}
	if strings.TrimSpace(tool.Command) == "" {
		if _, err := exec.LookPath(tool.Name); err != nil {
			return fail("not on PATH")
		}
	}
	output, code, err := command.Capture(root, commandText, timeout)
	if err != nil {
		if strings.HasPrefix(err.Error(), "command timed out after ") {
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
