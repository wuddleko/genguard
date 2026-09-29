package check

import (
	"bytes"
	"encoding/json"
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
	Tools  []jsonTool  `json:"tools,omitempty"`
}

type jsonTool struct {
	Name string `json:"name"`
	Want string `json:"want,omitempty"`
	Have string `json:"have,omitempty"`
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
	case cfg.Result.extra != nil:
		out.Error = oneLineError(cfg.Result.extra)
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
			item.Drifts = append(item.Drifts, jsonDrift{Kind: drift.Kind, Path: drift.Path})
		}
		if group.Status != GroupSkipped {
			for _, tool := range group.Tools {
				item.Tools = append(item.Tools, jsonTool{
					Name: tool.Name,
					Want: tool.Want,
					Have: tool.Have,
				})
			}
		}
		out.Groups = append(out.Groups, item)
	}
	return out
}
