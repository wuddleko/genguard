package check

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	return checkConfig(cfg, "", map[string]pathSnap{}, commandLog{})
}

func CheckSince(cfg config.Config, since string) (ConfigResult, error) {
	return checkSince(cfg, since, commandLog{})
}

func CheckSinceLog(cfg config.Config, since string, log io.Writer) (ConfigResult, error) {
	return checkSince(cfg, since, commandLog{w: log})
}

func checkSince(cfg config.Config, since string, log commandLog) (ConfigResult, error) {
	base, err := sinceBase(cfg, since)
	if err != nil {
		return ConfigResult{}, err
	}
	if base == "" {
		return checkConfig(cfg, "", map[string]pathSnap{}, log)
	}
	return checkGroups(cfg, base, map[string]pathSnap{}, log)
}

func sinceBase(cfg config.Config, since string) (string, error) {
	if strings.TrimSpace(since) == "" {
		return "", nil
	}
	if err := requireGitRepo(cfg.Root()); err != nil {
		return "", err
	}
	return mergeBase(cfg.Root(), since)
}

func checkConfig(cfg config.Config, base string, damage map[string]pathSnap, log commandLog) (ConfigResult, error) {
	if err := requireGitRepo(cfg.Root()); err != nil {
		return ConfigResult{}, err
	}
	return checkGroups(cfg, base, damage, log)
}

func checkGroups(cfg config.Config, base string, damage map[string]pathSnap, log commandLog) (ConfigResult, error) {
	if damage == nil {
		damage = map[string]pathSnap{}
	}
	root := cfg.Root()
	result := ConfigResult{}
	for _, group := range cfg.Groups {
		result.Groups = append(result.Groups, checkGroup(root, group, damage, base, cfg.Path, log))
	}
	return result, nil
}

func checkGroup(root string, group config.Group, damage map[string]pathSnap, base, configPath string, log commandLog) GroupResult {
	defer dropRepairedDamage(damage)

	var wipe map[string]pathSnap
	result := runPreparedGroup(root, group, base, configPath, log, func() {
		wipe = map[string]pathSnap{}
		recordCleanDamage(wipe, root, group)
	}, func(result GroupResult) GroupResult {
		found, driftErr := driftForGroup(root, group)
		if driftErr != nil {
			result.Err = driftErr
			return result
		}
		reported := omitUnchangedDamage(root, found, damage)
		reported = omitUnchangedDamage(root, reported, wipe)
		result.Drifts = reported
		if group.Clean {
			recordFoundDamage(damage, root, found)
		}
		return result
	})
	if result.Status != GroupOK {
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
	return result
}

type commandLog struct {
	w       io.Writer
	prefix  string
	timeout time.Duration
}

func (c commandLog) label(name string) string {
	text := name + ":"
	if c.prefix != "" {
		text = c.prefix + text
	}
	return text
}

func runPreparedGroup(root string, group config.Group, base, configPath string, log commandLog, afterClean func(), onCommandError func(GroupResult) GroupResult) GroupResult {
	result := GroupResult{Name: group.Name}
	if base != "" && len(group.Inputs) > 0 {
		affected, err := groupAffected(root, base, loadedConfigName(configPath), group)
		if err != nil {
			result.Status = GroupError
			result.Err = err
			return result
		}
		if !affected {
			result.Status = GroupSkipped
			return result
		}
	}

	if group.Clean {
		if err := cleanOutputs(root, configPath, group); err != nil {
			result.Status = GroupError
			result.Err = err
			return result
		}
		if afterClean != nil {
			afterClean()
		}
	}

	var header string
	if log.w != nil {
		header = log.label(group.Name)
	}
	tail, err := runCommand(root, group.Command, log.w, header, log.timeout)
	if err != nil {
		result.Status = GroupError
		result.CommandTail = tail
		if group.Clean {
			result.Err = newGenguardError("command failed after cleaning outputs: %s", err.Error())
		} else {
			result.Err = err
		}
		if onCommandError != nil {
			return onCommandError(result)
		}
		return result
	}
	result.Status = GroupOK
	return result
}

type pathSnap struct {
	missing bool
	mode    os.FileMode
	size    int64
	sum     [32]byte
	hashed  bool
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

func RunCommand(root, command string) (string, error) {
	return runCommand(root, command, nil, "", 0)
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

// rev-parse --show-toplevel resolves symlinks; keep the caller's spelling.
func callerRepoRoot(start, gitRoot string) string {
	resolvedStart, err := filepath.EvalSymlinks(start)
	if err != nil {
		return filepath.Clean(gitRoot)
	}
	resolvedRoot, err := filepath.EvalSymlinks(gitRoot)
	if err != nil {
		return filepath.Clean(gitRoot)
	}
	rel, ok := relInside(resolvedRoot, resolvedStart)
	if !ok {
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

func runCommand(root, command string, log io.Writer, header string, timeout time.Duration) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", newGenguardError("command is empty")
	}

	name, args := shellInvocation(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	var ring tailRing
	// A streamed log on Actions is bracketed with stop-commands. The token is
	// chosen before the child runs, so a rand failure skips the stream.
	stream := log != nil
	var token string
	var paused bool
	if stream && os.Getenv("GITHUB_ACTIONS") == "true" {
		var tokenErr error
		token, tokenErr = workflowStopToken()
		if tokenErr != nil {
			stream = false
		}
	}
	// headerWrote is set from onLine. The blank line follows the resume token.
	var headerWrote bool
	defer func() {
		if paused {
			fmt.Fprintf(log, "::%s::\n", token)
		}
		if headerWrote {
			io.WriteString(log, "\n")
		}
	}()
	if stream {
		var stopFailed bool
		ring.onLine = func(line string) {
			if stopFailed {
				return
			}
			if token != "" && !paused {
				n, werr := fmt.Fprintf(log, "::stop-commands::%s\n", token)
				if n > 0 {
					paused = true
				}
				if werr != nil {
					stopFailed = true
					stream = false
					return
				}
			}
			// The label shares the first streamed line, inside the pause.
			if header != "" && !headerWrote {
				if _, werr := fmt.Fprintln(log, header); werr != nil {
					stopFailed = true
					stream = false
					return
				}
				headerWrote = true
			}
			fmt.Fprintln(log, line)
		}
	}
	cmd.Stdout = &ring
	cmd.Stderr = &ring
	// A positive timeout puts the shell in its own process group so the
	// generator, a grandchild, dies with it. The timer starts after Start.
	var err error
	var timedOut bool
	if timeout > 0 {
		setCommandGroup(cmd)
		err = cmd.Start()
		if err == nil {
			wait := make(chan error, 1)
			go func() { wait <- cmd.Wait() }()
			timer := time.NewTimer(timeout)
			select {
			case err = <-wait:
				if !timer.Stop() {
					<-timer.C
				}
			case <-timer.C:
				timedOut = true
				stopCommand(cmd)
				err = <-wait
			}
		}
	} else {
		err = cmd.Run()
	}
	ring.flush()
	if timedOut {
		msg := newGenguardError("command timed out after %s", timeout)
		if log != nil && stream {
			return "", msg
		}
		return ring.String(), msg
	}
	if err == nil {
		return "", nil
	}

	exitCode := 1
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	tail := ring.String()
	if tail == "" {
		return "", newGenguardError("command failed (exit %d): no output", exitCode)
	}
	if log != nil && stream {
		return "", newGenguardError("command failed (exit %d)", exitCode)
	}
	return tail, newGenguardError("command failed (exit %d)", exitCode)
}

func workflowStopToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func errorsAsExit(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}

// driftPath is the config-relative spelling used as a drift key.
// Clean collapses ./hello.txt and foo/../hello.txt onto hello.txt, which is what git reports.
func driftPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func driftForGroup(root string, group config.Group) ([]Drift, error) {
	found := make([]Drift, 0)
	seen := make(map[string]struct{})
	record := func(path, kind string) {
		path = driftPath(path)
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		found = append(found, Drift{Group: group.Name, Path: path, Kind: kind})
	}

	modified, err := gitDiffNames(root, "HEAD", group.Outputs)
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
	untracked, err := gitUntracked(root, group.Outputs)
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
		if literalOutputAbsent(root, spec) {
			record(spec, "missing")
		}
	}

	return found, nil
}

func mergeBase(root, since string) (string, error) {
	since = strings.TrimSpace(since)
	if since == "" {
		return "", newGenguardError("--since requires a ref")
	}
	out, code, err := git(root, "merge-base", "HEAD", since)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", newGenguardError("bad --since ref: %s", gitDetail(out, "git merge-base failed"))
	}
	base := strings.TrimSpace(out)
	if base == "" {
		return "", newGenguardError("bad --since ref: empty merge-base")
	}
	return base, nil
}

func loadedConfigName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == ".." {
		return ""
	}
	return name
}

func groupAffected(root, base, configName string, group config.Group) (bool, error) {
	for _, spec := range group.Outputs {
		if literalOutputAbsent(root, spec) {
			return true, nil
		}
	}
	specs := make([]string, 0, len(group.Inputs)+len(group.Outputs)+1)
	specs = append(specs, group.Inputs...)
	specs = append(specs, group.Outputs...)
	if configName != "" {
		specs = append(specs, configName)
	}
	for _, rev := range []string{base, "HEAD"} {
		names, err := gitDiffNames(root, rev, specs)
		if err != nil || len(names) > 0 {
			return len(names) > 0, err
		}
	}
	untracked, err := gitUntracked(root, specs)
	return len(untracked) > 0, err
}

func literalOutputAbsent(root, spec string) bool {
	if isGlob(spec) || strings.HasSuffix(spec, "/") {
		return false
	}
	_, err := os.Stat(filepath.Join(root, spec))
	return err != nil
}

// --relative hides paths outside this directory, so names stay repo-root paths.
func gitDiffNames(root, rev string, specs []string) ([]string, error) {
	args := append([]string{"-c", "diff.relative=false", "diff", "--name-only", "-z", rev, "--"}, specs...)
	return gitNames(root, args...)
}

func gitUntracked(root string, specs []string) ([]string, error) {
	args := append([]string{"ls-files", "--others", "--exclude-standard", "-z", "--"}, specs...)
	return gitNames(root, args...)
}

func gitDiffText(root string, args ...string) (string, error) {
	full := append([]string{"-c", "diff.relative=false", "diff", "--no-color"}, args...)
	out, code, err := git(root, full...)
	if err != nil {
		return "", err
	}
	if code != 0 && code != 1 {
		return "", newGenguardError("%s", gitDetail(out, "git diff failed"))
	}
	return out, nil
}

func gitNames(root string, args ...string) ([]string, error) {
	out, code, err := git(root, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, newGenguardError("%s", gitDetail(out, "git failed"))
	}
	return parseGitNameList(out), nil
}

func gitPrefix(root string) (string, error) {
	out, code, err := git(root, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", newGenguardError("%s", gitDetail(out, "git rev-parse --show-prefix failed"))
	}
	return strings.TrimSpace(out), nil
}

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

func gitDetail(out, fallback string) string {
	detail := strings.TrimSpace(out)
	if detail == "" {
		return fallback
	}
	return detail
}

func git(root string, args ...string) (string, int, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return gitResult(stdout.String(), stderr.String(), err)
}

func gitResult(stdout, stderr string, err error) (string, int, error) {
	if err == nil {
		return stdout, 0, nil
	}
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		code := exitErr.ExitCode()
		// Exit 1 with a patch is a diff. A CRLF warning on stderr must not join it.
		if code == 1 {
			if strings.TrimSpace(stdout) == "" && strings.TrimSpace(stderr) != "" {
				return stderr, code, nil
			}
			return stdout, code, nil
		}
		if strings.TrimSpace(stderr) != "" {
			return stderr, code, nil
		}
		return stdout, code, nil
	}
	if strings.TrimSpace(stderr) != "" {
		return stderr, -1, err
	}
	return stdout, -1, err
}

func isGlob(spec string) bool {
	return strings.ContainsAny(spec, globChars)
}
