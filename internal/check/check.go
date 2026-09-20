package check

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wuddleko/regen/internal/config"
)

const globChars = "*?[]"

type Drift struct {
	Group string
	Path  string
	Kind  string
}

type RegenError struct {
	msg string
}

func (e *RegenError) Error() string {
	return e.msg
}

func newRegenError(format string, args ...any) error {
	return &RegenError{msg: fmt.Sprintf(format, args...)}
}

func CheckConfig(cfg config.Config) ([]Drift, error) {
	root := cfg.Root()
	if err := requireGitRepo(root); err != nil {
		return nil, err
	}

	var drifts []Drift
	for _, group := range cfg.Groups {
		if group.Clean {
			if err := cleanOutputs(root, group); err != nil {
				return nil, err
			}
		}
		if err := runCommand(root, group.Command); err != nil {
			if group.Clean {
				return nil, newRegenError("command failed after cleaning outputs: %s", err.Error())
			}
			return nil, err
		}
		found, err := driftForGroup(root, group)
		if err != nil {
			return nil, err
		}
		drifts = append(drifts, found...)
	}
	return drifts, nil
}

func RequireGitRepo(root string) error {
	return requireGitRepo(root)
}

func RunCommand(root, command string) error {
	return runCommand(root, command)
}

func DriftForGroup(root string, group config.Group) ([]Drift, error) {
	return driftForGroup(root, group)
}

func DriftDiff(root string, drifts []Drift) (string, error) {
	parts := make([]string, 0, len(drifts))
	seen := make(map[string]struct{}, len(drifts))

	for _, item := range drifts {
		if _, ok := seen[item.Path]; ok {
			continue
		}
		seen[item.Path] = struct{}{}

		switch item.Kind {
		case "modified", "missing":
			diff, err := gitDiffText(root, "HEAD", "--", item.Path)
			if err != nil {
				return "", err
			}
			diff = strings.TrimSpace(diff)
			if diff != "" {
				parts = append(parts, diff)
			}
		case "untracked":
			target := filepath.Join(root, item.Path)
			info, err := os.Stat(target)
			if err != nil || !info.Mode().IsRegular() {
				parts = append(parts, fmt.Sprintf("Untracked generated file: %s", item.Path))
				continue
			}
			diff, err := gitDiffText(root, "--no-index", os.DevNull, item.Path)
			if err != nil {
				return "", err
			}
			diff = strings.TrimSpace(diff)
			if diff != "" {
				parts = append(parts, diff)
			} else {
				parts = append(parts, fmt.Sprintf("Untracked generated file: %s", item.Path))
			}
		}
	}

	return strings.Join(parts, "\n"), nil
}

func requireGitRepo(root string) error {
	out, code, err := git(root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return err
	}
	if code != 0 || strings.TrimSpace(out) != "true" {
		return newRegenError("%s is not a git work tree", root)
	}
	return nil
}

func runCommand(root, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return newRegenError("command is empty")
	}

	name, args := shellInvocation(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	detail := strings.TrimSpace(string(output))
	if detail == "" {
		detail = "no output"
	}
	exitCode := 1
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	return newRegenError("command failed (exit %d): %s", exitCode, detail)
}

func errorsAsExit(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}

func driftForGroup(root string, group config.Group) ([]Drift, error) {
	found := make([]Drift, 0)
	seen := make(map[string]struct{})
	record := func(path, kind string) {
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		found = append(found, Drift{Group: group.Name, Path: path, Kind: kind})
	}

	modified, err := gitNames(root, append([]string{"diff", "--name-only", "HEAD", "--"}, group.Outputs...)...)
	if err != nil {
		return nil, err
	}
	untracked, err := gitNames(
		root,
		append([]string{"ls-files", "--others", "--exclude-standard", "--"}, group.Outputs...)...,
	)
	if err != nil {
		return nil, err
	}

	for _, path := range modified {
		kind := "modified"
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			kind = "missing"
		}
		record(path, kind)
	}
	for _, path := range untracked {
		record(path, "untracked")
	}

	for _, spec := range group.Outputs {
		if isGlob(spec) || strings.HasSuffix(spec, "/") {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, spec)); err == nil {
			continue
		}
		record(spec, "missing")
	}

	return found, nil
}

func gitDiffText(root string, args ...string) (string, error) {
	full := append([]string{"diff", "--no-color"}, args...)
	out, code, err := git(root, full...)
	if err != nil {
		return "", err
	}
	if code != 0 && code != 1 {
		detail := strings.TrimSpace(out)
		if detail == "" {
			detail = "git diff failed"
		}
		return "", newRegenError("%s", detail)
	}
	return out, nil
}

func gitNames(root string, args ...string) ([]string, error) {
	out, code, err := git(root, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		detail := strings.TrimSpace(out)
		if detail == "" {
			detail = "git failed"
		}
		return nil, newRegenError("%s", detail)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

func git(root string, args ...string) (string, int, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), 0, nil
	}
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		return string(output), exitErr.ExitCode(), nil
	}
	return string(output), -1, err
}

func isGlob(spec string) bool {
	return strings.ContainsAny(spec, globChars)
}
