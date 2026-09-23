package check

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

const globChars = "*?[]"

type Drift struct {
	Group string
	Path  string
	Kind  string
}

type GenguardError struct {
	msg string
}

func (e *GenguardError) Error() string {
	return e.msg
}

func newGenguardError(format string, args ...any) error {
	return &GenguardError{msg: fmt.Sprintf(format, args...)}
}

func CheckConfig(cfg config.Config) (ConfigResult, error) {
	return checkConfig(cfg, map[string]pathSnap{})
}

func checkConfig(cfg config.Config, damage map[string]pathSnap) (ConfigResult, error) {
	root := cfg.Root()
	if err := requireGitRepo(root); err != nil {
		return ConfigResult{}, err
	}
	if damage == nil {
		damage = map[string]pathSnap{}
	}

	result := ConfigResult{}
	for _, group := range cfg.Groups {
		result.Groups = append(result.Groups, checkGroup(root, group, damage))
	}
	return result, nil
}

func checkGroup(root string, group config.Group, damage map[string]pathSnap) GroupResult {
	result := GroupResult{Name: group.Name}
	// Residue is exempt only while it still matches the failed clean.
	// Drop snapshots this group changes so a later wipe is that group's drift.
	defer dropRepairedDamage(damage)

	// wipe is the tree immediately after a successful clean, before the
	// command. A failed command still diffs. Paths that still match an
	// earlier group's residue, or this wipe, are left out. damage is
	// updated afterward with the residue after the command, for later groups.
	var wipe map[string]pathSnap
	if group.Clean {
		if err := cleanOutputs(root, group); err != nil {
			result.Status = GroupError
			result.Err = err
			return result
		}
		wipe = snapshotClean(root, group)
	}

	if err := runCommand(root, group.Command); err != nil {
		result.Status = GroupError
		if group.Clean {
			result.Err = newGenguardError("command failed after cleaning outputs: %s", err.Error())
		} else {
			result.Err = err
		}
		found, driftErr := driftForGroup(root, group)
		if driftErr != nil {
			result.Err = driftErr
			return result
		}
		// The report leaves out wipe residue. damage records the full list,
		// including that residue, for later groups.
		reported := omitUnchangedDamage(root, found, damage)
		reported = omitUnchangedDamage(root, reported, wipe)
		result.Drifts = reported
		if group.Clean {
			recordFoundDamage(damage, root, found)
		}
		return result
	}

	found, err := driftForGroup(root, group)
	if err != nil {
		result.Status = GroupError
		result.Err = err
		return result
	}
	found = omitUnchangedDamage(root, found, damage)
	if len(found) > 0 {
		result.Status = GroupDrift
		result.Drifts = found
		return result
	}

	result.Status = GroupOK
	return result
}

// pathSnap is the on-disk state of a path after a group wiped it and then
// failed. Later groups leave that residue out of their drift while it still
// matches. A group that changes the path drops the snapshot.
type pathSnap struct {
	missing bool
	mode    os.FileMode
	size    int64
	sum     [32]byte
	hashed  bool
}

func snapshotClean(root string, group config.Group) map[string]pathSnap {
	snap := map[string]pathSnap{}
	recordCleanDamage(snap, root, group)
	return snap
}

func recordCleanDamage(damage map[string]pathSnap, root string, group config.Group) {
	found, err := driftForGroup(root, group)
	if err != nil {
		return
	}
	recordFoundDamage(damage, root, found)
}

func recordFoundDamage(damage map[string]pathSnap, root string, found []Drift) {
	for _, item := range found {
		abs := absDriftPath(root, item.Path)
		damage[abs] = snapPath(abs)
	}
}

func omitUnchangedDamage(root string, found []Drift, damage map[string]pathSnap) []Drift {
	if len(damage) == 0 || len(found) == 0 {
		return found
	}
	kept := make([]Drift, 0, len(found))
	for _, item := range found {
		abs := absDriftPath(root, item.Path)
		snap, ok := damage[abs]
		if ok && samePathSnap(abs, snap) {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// dropRepairedDamage forgets residue a group has changed. Comparing to the
// original snapshot, not to this group's drift list, catches a restore that
// matches HEAD and would otherwise leave the exemption in place.
func dropRepairedDamage(damage map[string]pathSnap) {
	if len(damage) == 0 {
		return
	}
	for abs, snap := range damage {
		if !samePathSnap(abs, snap) {
			delete(damage, abs)
		}
	}
}

func absDriftPath(root, rel string) string {
	return filepath.Clean(filepath.Join(root, rel))
}

func snapPath(abs string) pathSnap {
	info, err := os.Lstat(abs)
	if err != nil {
		return pathSnap{missing: true}
	}
	snap := pathSnap{mode: info.Mode(), size: info.Size()}
	if !info.Mode().IsRegular() {
		return snap
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return snap
	}
	snap.sum = sha256.Sum256(data)
	snap.hashed = true
	return snap
}

func samePathSnap(abs string, snap pathSnap) bool {
	info, err := os.Lstat(abs)
	if snap.missing {
		return os.IsNotExist(err)
	}
	if err != nil || info.Mode() != snap.mode || info.Size() != snap.size {
		return false
	}
	if !snap.hashed {
		return !info.Mode().IsRegular()
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return false
	}
	return sha256.Sum256(data) == snap.sum
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
		return newGenguardError("%s is not a git work tree", root)
	}
	return nil
}

func gitRepoRoot(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		dir = wd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := requireGitRepo(abs); err != nil {
		return "", err
	}
	out, code, err := git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(out)
	if code != 0 || root == "" {
		return "", newGenguardError("%s is not a git work tree", abs)
	}
	return callerRepoRoot(abs, root), nil
}

// callerRepoRoot returns gitRoot, using start's path spelling when start
// is that toplevel or a directory inside it. git rev-parse --show-toplevel
// resolves symlinks (/var -> /private/var on macOS); discovery should keep
// the path the caller passed.
func callerRepoRoot(start, gitRoot string) string {
	resolvedStart, err := filepath.EvalSymlinks(start)
	if err != nil {
		return filepath.Clean(gitRoot)
	}
	resolvedRoot, err := filepath.EvalSymlinks(gitRoot)
	if err != nil {
		return filepath.Clean(gitRoot)
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedStart)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Clean(gitRoot)
	}
	if rel == "." {
		return filepath.Clean(start)
	}
	suffix := string(filepath.Separator) + rel
	if strings.HasSuffix(start, suffix) {
		return filepath.Clean(strings.TrimSuffix(start, suffix))
	}
	return filepath.Clean(gitRoot)
}

func runCommand(root, command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return newGenguardError("command is empty")
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
	return newGenguardError("command failed (exit %d): %s", exitCode, detail)
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

	// Repo-root names, not --relative. --relative also drops paths outside
	// the config directory, so a tracked edit at ../sibling/file.go would
	// pass. diff.relative is forced off in case the user has it set.
	modified, err := gitNames(root, append([]string{"-c", "diff.relative=false", "diff", "--name-only", "-z", "HEAD", "--"}, group.Outputs...)...)
	if err != nil {
		return nil, err
	}
	if len(modified) > 0 {
		prefix, err := gitPrefix(root)
		if err != nil {
			return nil, err
		}
		for i, path := range modified {
			rel, err := configRelativeGitPath(prefix, path)
			if err != nil {
				return nil, err
			}
			modified[i] = rel
		}
	}
	untracked, err := gitNames(
		root,
		append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, group.Outputs...)...,
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
	full := append([]string{"-c", "diff.relative=false", "diff", "--no-color"}, args...)
	out, code, err := git(root, full...)
	if err != nil {
		return "", err
	}
	if code != 0 && code != 1 {
		detail := strings.TrimSpace(out)
		if detail == "" {
			detail = "git diff failed"
		}
		return "", newGenguardError("%s", detail)
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
		return nil, newGenguardError("%s", detail)
	}
	return parseGitNameList(out), nil
}

func gitPrefix(root string) (string, error) {
	out, code, err := git(root, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	if code != 0 {
		detail := strings.TrimSpace(out)
		if detail == "" {
			detail = "git rev-parse --show-prefix failed"
		}
		return "", newGenguardError("%s", detail)
	}
	return strings.TrimSpace(out), nil
}

// configRelativeGitPath maps a repo-root path from git diff onto the config
// directory. prefix is git rev-parse --show-prefix: empty at the toplevel,
// "api/" when the config lives in api/.
func configRelativeGitPath(prefix, gitPath string) (string, error) {
	base := strings.TrimSuffix(prefix, "/")
	if base == "" {
		base = "."
	}
	rel, err := filepath.Rel(filepath.FromSlash(base), filepath.FromSlash(gitPath))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func parseGitNameList(out string) []string {
	if out == "" {
		return nil
	}
	parts := strings.Split(out, "\x00")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			names = append(names, part)
		}
	}
	return names
}

func git(root string, args ...string) (string, int, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return stdout.String(), 0, nil
	}
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		code := exitErr.ExitCode()
		// Exit 1 is a diff with changes. The patch is on stdout. A CRLF
		// warning on stderr must not be appended to it. A failure that
		// exits 1 with an empty stdout still has its message on stderr.
		if code == 1 {
			if strings.TrimSpace(stdout.String()) == "" && strings.TrimSpace(stderr.String()) != "" {
				return stderr.String(), code, nil
			}
			return stdout.String(), code, nil
		}
		if strings.TrimSpace(stderr.String()) != "" {
			return stderr.String(), code, nil
		}
		return stdout.String(), code, nil
	}
	if strings.TrimSpace(stderr.String()) != "" {
		return stderr.String(), -1, err
	}
	return stdout.String(), -1, err
}

func isGlob(spec string) bool {
	return strings.ContainsAny(spec, globChars)
}
