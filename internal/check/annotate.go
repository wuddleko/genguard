package check

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func FormatAnnotations(run RunResult) string {
	var b strings.Builder
	for _, cfg := range run.Configs {
		configFile := workspaceFile(run.RepoRoot, strings.ReplaceAll(displayConfigPath(run.RepoRoot, cfg.Path), "\\", "/"))
		if cfg.Err != nil {
			writeAnnotation(&b, configFile, "", oneLineError(cfg.Err))
			continue
		}
		for _, group := range cfg.Result.Groups {
			if group.Status != GroupDrift && group.Status != GroupError {
				continue
			}
			if group.Err != nil {
				writeAnnotation(&b, configFile, group.Name, oneLineError(group.Err))
			}
			for _, drift := range group.Drifts {
				writeAnnotation(&b, workspaceFile(run.RepoRoot, jsonDriftPath(run.RepoRoot, cfg.Path, drift.Path)), group.Name, drift.Kind)
			}
		}
		if cfg.Result.cleanup != nil {
			writeAnnotation(&b, configFile, "", oneLineError(cfg.Result.cleanup))
		}
	}
	return b.String()
}

func writeAnnotation(b *strings.Builder, file, title, message string) {
	b.WriteString("::error")
	if file != "" || title != "" {
		b.WriteString(" ")
		sep := ""
		if file != "" {
			b.WriteString("file=")
			b.WriteString(escapeProperty(file))
			sep = ","
		}
		if title != "" {
			b.WriteString(sep)
			b.WriteString("title=")
			b.WriteString(escapeProperty(title))
		}
	}
	b.WriteString("::")
	b.WriteString(escapeData(message))
	b.WriteString("\n")
}

// FormatErrorAnnotation is the ::error line for a failure that happens before
// any group result exists. file is a config path when there is one.
func FormatErrorAnnotation(file, message string) string {
	var b strings.Builder
	writeAnnotation(&b, errorAnnotationFile(file), "", oneLineError(errors.New(message)))
	return b.String()
}

func errorAnnotationFile(file string) string {
	if file == "" {
		return ""
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return filepath.ToSlash(file)
	}
	repoRoot, err := gitRepoRoot(filepath.Dir(abs))
	if err != nil {
		return filepath.ToSlash(abs)
	}
	return workspaceFile(repoRoot, filepath.ToSlash(displayConfigPath(repoRoot, abs)))
}

// workspaceFile returns file relative to GITHUB_WORKSPACE when the repository
// is inside it. GitHub resolves annotation paths from that directory.
func workspaceFile(repoRoot, file string) string {
	file = strings.ReplaceAll(file, "\\", "/")
	if file == "" || repoRoot == "" || filepath.IsAbs(file) {
		return file
	}
	workspace := strings.TrimSpace(os.Getenv("GITHUB_WORKSPACE"))
	if workspace == "" {
		return file
	}
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return file
	}
	wsAbs, err := filepath.Abs(workspace)
	if err != nil {
		return file
	}
	prefix, err := filepath.Rel(wsAbs, rootAbs)
	if err != nil || prefix == ".." || strings.HasPrefix(prefix, ".."+string(filepath.Separator)) {
		return file
	}
	if prefix == "." {
		return file
	}
	return filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(file)))
}

func escapeData(s string) string {
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	).Replace(s)
}

func escapeProperty(s string) string {
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
		":", "%3A",
		",", "%2C",
	).Replace(s)
}
