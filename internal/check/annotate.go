package check

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/actions"
)

func FormatAnnotations(run RunResult, env actions.Env) string {
	var b strings.Builder
	for _, cfg := range run.Configs {
		configFile := actions.WorkspaceFile(env.Workspace, run.RepoRoot, strings.ReplaceAll(displayConfigPath(run.RepoRoot, cfg.Path), "\\", "/"))
		if cfg.Err != nil {
			b.WriteString(actions.Annotation(configFile, "", oneLineError(cfg.Err)))
			continue
		}
		for _, group := range cfg.Result.Groups {
			if group.Status != GroupDrift && group.Status != GroupError {
				continue
			}
			if group.Err != nil {
				b.WriteString(actions.Annotation(configFile, group.Name, oneLineError(group.Err)))
			}
			for _, drift := range group.Drifts {
				file := actions.WorkspaceFile(env.Workspace, run.RepoRoot, drift.Path)
				b.WriteString(actions.Annotation(file, group.Name, drift.Kind))
			}
		}
		if cfg.Result.extra != nil {
			b.WriteString(actions.Annotation(configFile, "", oneLineError(cfg.Result.extra)))
		}
	}
	return b.String()
}

func FormatErrorAnnotation(file, message string, env actions.Env) string {
	return actions.Annotation(errorAnnotationFile(file, env), "", oneLineError(errors.New(message)))
}

func errorAnnotationFile(file string, env actions.Env) string {
	if file == "" {
		return ""
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return filepath.ToSlash(file)
	}
	repoRoot, err := gitRepoRoot(commandLog{}, filepath.Dir(abs))
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return actions.WorkspaceFile(env.Workspace, repoRoot, filepath.ToSlash(displayConfigPath(repoRoot, abs)))
}
