package check

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/actions"
	"github.com/wuddleko/genguard/internal/check/clean"
	"github.com/wuddleko/genguard/internal/command"
	"github.com/wuddleko/genguard/internal/config"
)

type Mode int

const (
	ModeCheck Mode = iota
	ModeRun
)

type Options struct {
	Mode     Mode
	Context  context.Context
	Since    string
	Isolated bool
	Log      io.Writer
	Quiet    bool
	Env      actions.Env
	// RepoRoot is the repository, found from the config directory, or for
	// ExecuteAll the working directory, when empty. Paths replaces discovery
	// in ExecuteAll.
	RepoRoot string
	Paths    []string
}

var errRunIsolated = errors.New("genguard run writes the checkout; --isolated is not valid")

// Execute runs one config. The error is set, and the result empty, when
// nothing ran.
func Execute(opts Options, configPath string) (RunResult, error) {
	if opts.Mode == ModeRun && opts.Isolated {
		return RunResult{}, errRunIsolated
	}
	log := commandLog{w: opts.Log, quiet: opts.Quiet, ctx: opts.Context, env: opts.Env}
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return RunResult{}, err
	}
	var cfg config.Config
	if !opts.Isolated {
		if cfg, err = config.LoadConfig(configPath); err != nil {
			return RunResult{}, err
		}
	}
	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		if repoRoot, err = gitRepoRoot(log, filepath.Dir(abs)); err != nil {
			return RunResult{}, err
		}
	}
	base, err := sinceBase(log, repoRoot, opts.Since)
	if err != nil {
		return RunResult{}, err
	}
	var result ConfigResult
	if opts.Isolated {
		result, err = checkIsolated(log, repoRoot, configPath, base)
	} else {
		result, err = groups(cfg, repoRoot, base, log, opts.Mode)
	}
	if err != nil && len(result.Groups) == 0 {
		return RunResult{}, err
	}
	return RunResult{RepoRoot: repoRoot, Configs: []ConfigRun{{Path: abs, Result: result}}}, nil
}

func sinceBase(log commandLog, repoRoot, since string) (string, error) {
	if strings.TrimSpace(since) == "" {
		return "", nil
	}
	return mergeBase(log, repoRoot, since)
}

// configRunner is what the groups of one config share.
type configRunner struct {
	cfg    config.Config
	tree   tree
	base   string
	log    commandLog
	mode   Mode
	probes toolCache
}

// groups returns an error only when the run was interrupted before any group.
func groups(cfg config.Config, repoRoot, base string, log commandLog, mode Mode) (ConfigResult, error) {
	t, err := newTree(repoRoot, cfg.Root())
	if err != nil {
		return ConfigResult{}, err
	}
	r := configRunner{cfg: cfg, tree: t, base: base, log: log, mode: mode, probes: toolCache{}}
	result := ConfigResult{}
	for _, group := range cfg.Groups {
		if log.canceled() {
			break
		}
		result.Groups = append(result.Groups, r.group(group))
	}
	if !log.canceled() {
		return result, nil
	}
	if len(result.Groups) == 0 {
		return result, command.ErrInterrupted
	}
	if !result.hasInterrupt() {
		result.note(command.ErrInterrupted)
	}
	return result, nil
}

func (r configRunner) group(group config.Group) GroupResult {
	root := r.tree.dir
	log := r.log
	result := GroupResult{Name: group.Name}
	if r.base != "" && len(group.Inputs) > 0 {
		affected, err := groupAffected(log, root, r.base, loadedConfigName(r.cfg.Path), group)
		if err != nil {
			return result.failed(err)
		}
		if !affected {
			if log.canceled() {
				return result.failed(command.ErrInterrupted)
			}
			result.Status = GroupSkipped
			return result
		}
	}

	defer log.beginGroup(group.Name)()

	if len(group.Tools) > 0 {
		observed, err := verifyTools(root, r.cfg.Tools, group.Tools, group.Timeout, log, r.probes)
		result.Tools = observed
		if err != nil {
			return result.failed(err)
		}
	}

	var wipe map[string]pathSnap
	if group.Clean {
		if err := clean.Outputs(log.ctx, root, r.cfg.Path, group); err != nil {
			return result.failed(err)
		}
		if r.mode == ModeCheck {
			wipe = map[string]pathSnap{}
			if err := recordCleanDamage(log, wipe, r.tree, group); err != nil {
				return result.failed(err)
			}
		}
	}

	stream := log.w
	if log.quiet {
		stream = nil
	}
	var header string
	if stream != nil {
		header = log.label(group.Name)
	}
	tail, err := runCommandContext(log.ctx, root, group.Command, stream, header, group.Timeout, log.env)
	if log.quiet && log.groups() && tail != "" {
		log.writeGroupedTail(group.Name, tail)
		tail = ""
	}
	if err != nil {
		if group.Clean {
			err = fmt.Errorf("command failed after cleaning outputs: %w", err)
		}
		result = result.failed(err)
		result.CommandTail = tail
		if r.mode == ModeRun {
			return result
		}
		// No context: an interrupted command still reports what it left behind.
		found, driftErr := driftForGroup(commandLog{}, r.tree, group)
		if driftErr != nil {
			result.Err = fmt.Errorf("%w: %v", result.Err, driftErr)
			return result
		}
		result.Drifts = omitUnchangedDamage(r.tree, found, wipe)
		return result
	}

	result.Status = GroupOK
	if r.mode == ModeRun {
		return result
	}
	found, err := driftForGroup(log, r.tree, group)
	if err != nil {
		return result.failed(err)
	}
	if len(found) > 0 {
		result.Status = GroupDrift
		result.Drifts = found
	}
	return result
}

func (g GroupResult) failed(err error) GroupResult {
	g.Status = GroupError
	g.Err = err
	return g
}

func isInterrupt(err error) bool {
	return errors.Is(err, command.ErrInterrupted)
}

func loadedConfigName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == ".." {
		return ""
	}
	return name
}
