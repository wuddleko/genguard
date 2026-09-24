package check

import (
	"bytes"
	"encoding/json"
	"path/filepath"
)

type jsonRun struct {
	Exit    int          `json:"exit"`
	Configs []jsonConfig `json:"configs"`
}

type jsonConfig struct {
	Path   string      `json:"path"`
	Exit   int         `json:"exit"`
	Error  string      `json:"error,omitempty"`
	Groups []jsonGroup `json:"groups"`
}

type jsonGroup struct {
	Name   string      `json:"name"`
	Status string      `json:"status"`
	Error  string      `json:"error,omitempty"`
	Drifts []jsonDrift `json:"drifts,omitempty"`
}

type jsonDrift struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

func FormatJSON(run RunResult) (string, error) {
	doc := jsonRun{
		Exit:    run.ExitCode(),
		Configs: make([]jsonConfig, 0, len(run.Configs)),
	}
	for _, cfg := range run.Configs {
		doc.Configs = append(doc.Configs, jsonConfigFrom(run.RepoRoot, cfg))
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func jsonConfigFrom(repoRoot string, cfg ConfigRun) jsonConfig {
	out := jsonConfig{
		Path:   displayConfigPath(repoRoot, cfg.Path),
		Exit:   cfg.ExitCode(),
		Groups: make([]jsonGroup, 0, len(cfg.Result.Groups)),
	}
	switch {
	case cfg.Err != nil:
		out.Error = oneLineError(cfg.Err)
	case cfg.Result.cleanup != nil:
		out.Error = oneLineError(cfg.Result.cleanup)
	}
	for _, group := range cfg.Result.Groups {
		item := jsonGroup{
			Name:   group.Name,
			Status: string(group.Status),
		}
		if group.Err != nil {
			item.Error = oneLineError(group.Err)
		}
		for _, drift := range group.Drifts {
			item.Drifts = append(item.Drifts, jsonDrift{
				Kind: drift.Kind,
				Path: jsonDriftPath(repoRoot, cfg.Path, drift.Path),
			})
		}
		out.Groups = append(out.Groups, item)
	}
	return out
}

// jsonDriftPath prints a drift relative to the repository root.
// Stored drift paths are relative to the config directory.
func jsonDriftPath(repoRoot, configPath, driftPath string) string {
	if repoRoot == "" || driftPath == "" || !filepath.IsAbs(configPath) {
		return driftPath
	}
	abs := filepath.Join(filepath.Dir(configPath), filepath.FromSlash(driftPath))
	return filepath.ToSlash(displayConfigPath(repoRoot, abs))
}
