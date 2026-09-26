package check

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

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

func CheckConfig(cfg config.Config) (ConfigResult, error) {
	return checkConfig(cfg, "", map[string]pathSnap{}, commandLog{})
}

func CheckSince(cfg config.Config, since string) (ConfigResult, error) {
	return checkSince(cfg, since, commandLog{})
}

func CheckSinceLog(cfg config.Config, since string, log io.Writer, quiet bool) (ConfigResult, error) {
	return checkSince(cfg, since, commandLog{w: log, quiet: quiet})
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

	defer log.beginGroup(group.Name)()

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

	stream := log.w
	if log.quiet {
		stream = nil
	}
	var header string
	if stream != nil {
		header = log.label(group.Name)
	}
	tail, err := runCommand(root, group.Command, stream, header, log.timeout)
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

func loadedConfigName(path string) string {
	name := filepath.Base(path)
	if name == "." || name == ".." {
		return ""
	}
	return name
}
