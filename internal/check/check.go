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

var errInterrupted = errors.New("interrupted")

func isInterrupt(err error) bool {
	return errors.Is(err, errInterrupted) || errors.Is(err, command.ErrInterrupted)
}

func CheckConfig(cfg config.Config) (ConfigResult, error) {
	return executeConfig(cfg, Options{Mode: ModeCheck})
}

func CheckSince(cfg config.Config, since string) (ConfigResult, error) {
	return executeConfig(cfg, Options{Mode: ModeCheck, Since: since})
}

func CheckSinceLog(ctx context.Context, cfg config.Config, since string, log io.Writer, quiet bool, env actions.Env) (ConfigResult, error) {
	return executeConfig(cfg, Options{Mode: ModeCheck, Since: since, Log: log, Quiet: quiet, Context: ctx, Env: env})
}

func executeConfig(cfg config.Config, opts Options) (ConfigResult, error) {
	log := commandLog{w: opts.Log, quiet: opts.Quiet, ctx: opts.Context, env: opts.Env}
	return sinceConfig(cfg, opts.Since, log, opts.Mode)
}

func checkSince(cfg config.Config, since string, log commandLog) (ConfigResult, error) {
	return sinceConfig(cfg, since, log, ModeCheck)
}

func sinceConfig(cfg config.Config, since string, log commandLog, mode Mode) (ConfigResult, error) {
	base, err := sinceBase(log, cfg, since)
	if err != nil {
		return ConfigResult{}, err
	}
	if base == "" {
		if mode == ModeRun {
			return runConfig(cfg, "", log)
		}
		return checkConfig(cfg, "", log)
	}
	if mode == ModeRun {
		return groups(cfg, base, log, ModeRun)
	}
	return groups(cfg, base, log, ModeCheck)
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

func checkConfig(cfg config.Config, base string, log commandLog) (ConfigResult, error) {
	if err := requireGitRepo(log, cfg.Root()); err != nil {
		return ConfigResult{}, err
	}
	return groups(cfg, base, log, ModeCheck)
}

func runConfig(cfg config.Config, base string, log commandLog) (ConfigResult, error) {
	if err := requireGitRepo(log, cfg.Root()); err != nil {
		return ConfigResult{}, err
	}
	return groups(cfg, base, log, ModeRun)
}

func groups(cfg config.Config, base string, log commandLog, mode Mode) (ConfigResult, error) {
	log = log.withToolCache()
	root := cfg.Root()
	result := ConfigResult{}
	for _, group := range cfg.Groups {
		if stop, err := canceledStop(log.ctx, result.ExitCode(), result.noteCleanup); stop {
			return result, err
		}
		var groupResult GroupResult
		if mode == ModeCheck {
			groupResult = checkGroup(root, group, cfg.Tools, base, cfg.Path, log)
		} else {
			groupResult = runPreparedGroup(root, group, cfg.Tools, base, cfg.Path, log, nil, nil)
		}
		result.Groups = append(result.Groups, groupResult)
	}
	if stop, err := canceledStop(log.ctx, result.ExitCode(), result.noteCleanup); stop {
		return result, err
	}
	return result, nil
}

func checkGroup(root string, group config.Group, tools []config.Tool, base, configPath string, log commandLog) GroupResult {
	var wipe map[string]pathSnap
	result := runPreparedGroup(root, group, tools, base, configPath, log, func() error {
		wipe = map[string]pathSnap{}
		return recordCleanDamage(log, wipe, root, group)
	}, func(result GroupResult) GroupResult {
		found, driftErr := driftForGroup(commandLog{}, root, group)
		if driftErr != nil {
			result.Err = newGenguardError("%s: %s", result.Err.Error(), driftErr.Error())
			return result
		}
		result.Drifts = omitUnchangedDamage(root, found, wipe)
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
	tail, err := runCommandContext(log.ctx, root, group.Command, stream, header, limit, log.env)
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
