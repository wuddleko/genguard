package check

import (
	"strings"

	"github.com/wuddleko/genguard/internal/config"
)

func RunConfig(cfg config.Config) (ConfigResult, error) {
	return runConfig(cfg, "")
}

func RunSince(cfg config.Config, since string) (ConfigResult, error) {
	if strings.TrimSpace(since) == "" {
		return RunConfig(cfg)
	}
	root := cfg.Root()
	if err := requireGitRepo(root); err != nil {
		return ConfigResult{}, err
	}
	base, err := mergeBase(root, since)
	if err != nil {
		return ConfigResult{}, err
	}
	return runConfig(cfg, base)
}

func runConfig(cfg config.Config, base string) (ConfigResult, error) {
	root := cfg.Root()
	if err := requireGitRepo(root); err != nil {
		return ConfigResult{}, err
	}
	result := ConfigResult{}
	for _, group := range cfg.Groups {
		result.Groups = append(result.Groups, runGroup(root, group, base))
	}
	return result, nil
}

func runGroup(root string, group config.Group, base string) GroupResult {
	result := GroupResult{Name: group.Name}
	if base != "" && len(group.Inputs) > 0 {
		affected, err := groupAffected(root, base, group)
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
		if err := cleanOutputs(root, group); err != nil {
			result.Status = GroupError
			result.Err = err
			return result
		}
	}

	if err := runCommand(root, group.Command); err != nil {
		result.Status = GroupError
		if group.Clean {
			result.Err = newGenguardError("command failed after cleaning outputs: %s", err.Error())
		} else {
			result.Err = err
		}
		return result
	}

	result.Status = GroupOK
	return result
}
