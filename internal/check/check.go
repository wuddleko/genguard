package check

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/check/clean"
	"github.com/wuddleko/genguard/internal/check/command"
	"github.com/wuddleko/genguard/internal/config"
)

type GenguardError struct {
	msg string
}

func (e *GenguardError) Error() string {
	return e.msg
}

func newGenguardError(format string, args ...any) error {
	return &GenguardError{msg: fmt.Sprintf(format, args...)}
}

// errInterrupted is the run's cancel error. Command failures keep their own text.
var errInterrupted = errors.New("interrupted")

func isInterrupt(err error) bool {
	return errors.Is(err, errInterrupted) || errors.Is(err, command.ErrInterrupted)
}

func CheckConfig(cfg config.Config) (ConfigResult, error) {
	return checkConfig(cfg, "", map[string]pathSnap{}, commandLog{})
}

func CheckSince(cfg config.Config, since string) (ConfigResult, error) {
	return checkSince(cfg, since, commandLog{})
}

func CheckSinceLog(ctx context.Context, cfg config.Config, since string, log io.Writer, quiet bool) (ConfigResult, error) {
	return checkSince(cfg, since, commandLog{w: log, quiet: quiet, ctx: ctx})
}

func checkSince(cfg config.Config, since string, log commandLog) (ConfigResult, error) {
	base, err := sinceBase(log, cfg, since)
	if err != nil {
		return ConfigResult{}, err
	}
	if base == "" {
		return checkConfig(cfg, "", map[string]pathSnap{}, log)
	}
	return checkGroups(cfg, base, map[string]pathSnap{}, log)
}

func sinceBase(log commandLog, cfg config.Config, since string) (string, error) {
	if strings.TrimSpace(since) == "" {
		return "", nil
	}
	if err := requireGitRepo(log, cfg.Root()); err != nil {
		return "", err
	}
	return mergeBase(log, cfg.Root(), since)
}

func checkConfig(cfg config.Config, base string, damage map[string]pathSnap, log commandLog) (ConfigResult, error) {
	if err := requireGitRepo(log, cfg.Root()); err != nil {
		return ConfigResult{}, err
	}
	return checkGroups(cfg, base, damage, log)
}

func checkGroups(cfg config.Config, base string, damage map[string]pathSnap, log commandLog) (ConfigResult, error) {
	if damage == nil {
		damage = map[string]pathSnap{}
	}
	log = log.withToolCache()
	root := cfg.Root()
	result := ConfigResult{}
	for _, group := range cfg.Groups {
		if stop, err := canceledStop(log.ctx, result.ExitCode(), result.noteCleanup); stop {
			return result, err
		}
		result.Groups = append(result.Groups, checkGroup(root, group, cfg.Tools, damage, base, cfg.Path, log))
	}
	if stop, err := canceledStop(log.ctx, result.ExitCode(), result.noteCleanup); stop {
		return result, err
	}
	return result, nil
}

func checkGroup(root string, group config.Group, tools []config.Tool, damage map[string]pathSnap, base, configPath string, log commandLog) GroupResult {
	defer dropRepairedDamage(damage)

	var wipe map[string]pathSnap
	result := runPreparedGroup(root, group, tools, base, configPath, log, func() error {
		wipe = map[string]pathSnap{}
		return recordCleanDamage(log, wipe, root, group)
	}, func(result GroupResult) GroupResult {
		// The command already failed. Diff anyway, even when the context is
		// already canceled, so the report still shows what changed.
		found, driftErr := driftForGroup(commandLog{}, root, group)
		if driftErr != nil {
			result.Err = newGenguardError("%s: %s", result.Err.Error(), driftErr.Error())
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

	found, err := driftForGroup(log, root, group)
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

func runPreparedGroup(root string, group config.Group, tools []config.Tool, base, configPath string, log commandLog, afterClean func() error, onCommandError func(GroupResult) GroupResult) GroupResult {
	result := GroupResult{Name: group.Name}
	if base != "" && len(group.Inputs) > 0 {
		affected, err := groupAffected(log, root, base, loadedConfigName(configPath), group)
		if err != nil {
			result.Status = GroupError
			result.Err = err
			return result
		}
		if !affected {
			// Nothing in the group runs after this, so a cancel here would
			// otherwise be reported as a skip.
			if log.canceled() {
				result.Status = GroupError
				result.Err = errInterrupted
				return result
			}
			result.Status = GroupSkipped
			return result
		}
	}

	defer log.beginGroup(group.Name)()

	// A configured timeout wins. commandLog.timeout injects one when the group has none.
	limit := group.Timeout
	if limit == 0 {
		limit = log.timeout
	}

	if len(group.Tools) > 0 {
		observed, err := verifyTools(root, tools, group.Tools, limit, log)
		result.Tools = observed
		if err != nil {
			result.Status = GroupError
			result.Err = err
			return result
		}
	}

	if group.Clean {
		if err := clean.Outputs(log.ctx, root, configPath, group); err != nil {
			result.Status = GroupError
			if isInterrupt(err) {
				result.Err = errInterrupted
			} else {
				result.Err = newGenguardError("%s", err.Error())
			}
			return result
		}
		if afterClean != nil {
			// The wipe is already done. Without a snapshot the generator must not run.
			if err := afterClean(); err != nil {
				result.Status = GroupError
				result.Err = err
				return result
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
	tail, err := runCommandContext(log.ctx, root, group.Command, stream, header, limit)
	if log.quiet && log.groups() && tail != "" {
		log.writeGroupedTail(group.Name, tail)
		tail = ""
	}
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

// canceledStop reports that the walk should end.
// Exit 2 is already a failure, so that result stays as it is.
// Exit 1 is a finished drift. keep records the interrupt on that result so
// the drift is still reported and the run exits 2.
// A cancel before any failure is the error.
func canceledStop(ctx context.Context, code int, keep func(error)) (bool, error) {
	if ctx == nil || ctx.Err() == nil {
		return false, nil
	}
	if code == 2 {
		return true, nil
	}
	err := errInterrupted
	if code == 1 && keep != nil {
		keep(err)
		return true, nil
	}
	return true, err
}

// noteInterruptedDrift marks configs that already drifted, so a cancel between
// configs still reports that drift and exits 2.
func noteInterruptedDrift(run *RunResult, err error) {
	for i := range run.Configs {
		cfg := &run.Configs[i]
		if cfg.Err == nil && cfg.Result.ExitCode() == 1 {
			cfg.Result.noteCleanup(err)
		}
	}
}

func loadedConfigName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == ".." {
		return ""
	}
	return name
}
