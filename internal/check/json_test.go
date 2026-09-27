package check

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatJSONDriftOmitsDiff(t *testing.T) {
	root := t.TempDir()
	run := RunResult{
		RepoRoot: root,
		Configs: []ConfigRun{{
			Path: filepath.Join(root, "api", "genguard.yaml"),
			Result: ConfigResult{Groups: []GroupResult{
				{Name: "protobuf", Status: GroupOK},
				{Name: "sqlc", Status: GroupDrift, Drifts: []Drift{
					{Group: "sqlc", Kind: "modified", Path: "gen/a.go"},
					{Group: "sqlc", Kind: "untracked", Path: "gen/b.go"},
					{Group: "sqlc", Kind: "missing", Path: "../top.txt"},
				}},
			}},
		}},
	}
	text := mustFormatJSON(t, run)
	if strings.Contains(text, "diff --git") || strings.Contains(text, "Summary") {
		t.Fatalf("json = %s", text)
	}
	doc := decodeJSON(t, text)
	if doc.Exit != 1 || len(doc.Configs) != 1 {
		t.Fatalf("doc = %+v", doc)
	}
	cfg := doc.Configs[0]
	if cfg.Path != filepath.Join("api", "genguard.yaml") || cfg.Exit != 1 {
		t.Fatalf("config = %+v", cfg)
	}
	if cfg.Groups[0].Status != "ok" || len(cfg.Groups[0].Drifts) != 0 {
		t.Fatalf("ok group = %+v", cfg.Groups[0])
	}
	drift := cfg.Groups[1]
	if drift.Name != "sqlc" || drift.Status != "drift" || drift.Error != "" {
		t.Fatalf("drift group = %+v", drift)
	}
	if len(drift.Drifts) != 3 || drift.Drifts[0].Kind != "modified" || drift.Drifts[0].Path != "api/gen/a.go" {
		t.Fatalf("drifts = %+v", drift.Drifts)
	}
	if drift.Drifts[1].Path != "api/gen/b.go" || drift.Drifts[2].Path != "top.txt" {
		t.Fatalf("drifts = %+v", drift.Drifts)
	}
}

func TestFormatJSONErrorFlattensAndKeepsDriftPaths(t *testing.T) {
	run := RunResult{Configs: []ConfigRun{{
		Path: "genguard.yaml",
		Result: ConfigResult{Groups: []GroupResult{{
			Name:        "protobuf",
			Status:      GroupError,
			Err:         errors.New("command failed (exit 1): line1\nline2"),
			CommandTail: "kept-tail\n",
			Drifts:      []Drift{{Kind: "modified", Path: "gen/a.go"}},
		}}},
	}}}
	text := mustFormatJSON(t, run)
	if strings.Contains(text, "kept-tail") || strings.Contains(text, "commandTail") {
		t.Fatalf("json = %s", text)
	}
	doc := decodeJSON(t, text)
	if doc.Exit != 2 {
		t.Fatalf("exit = %d", doc.Exit)
	}
	group := doc.Configs[0].Groups[0]
	if group.Error != "command failed (exit 1): line1 line2" {
		t.Fatalf("error = %q", group.Error)
	}
	if len(group.Drifts) != 1 || group.Drifts[0].Path != "gen/a.go" {
		t.Fatalf("drifts = %+v", group.Drifts)
	}
}

func TestFormatJSONSkippedAndLoadError(t *testing.T) {
	root := t.TempDir()
	run := RunResult{
		RepoRoot: root,
		Configs: []ConfigRun{
			{
				Path: filepath.Join(root, "genguard.yaml"),
				Result: ConfigResult{Groups: []GroupResult{
					{Name: "sqlc", Status: GroupSkipped},
				}},
			},
			{
				Path: filepath.Join(root, "api", "genguard.yaml"),
				Err:  errors.New("parse api/genguard.yaml: bad"),
			},
		},
	}
	doc := decodeJSON(t, mustFormatJSON(t, run))
	if doc.Exit != 2 || doc.Configs[0].Exit != 0 || doc.Configs[0].Groups[0].Status != "skipped" {
		t.Fatalf("doc = %+v", doc)
	}
	if len(doc.Configs[0].Groups[0].Drifts) != 0 {
		t.Fatalf("skipped drifts = %+v", doc.Configs[0].Groups[0].Drifts)
	}
	failed := doc.Configs[1]
	if failed.Exit != 2 || failed.Error != "parse api/genguard.yaml: bad" || len(failed.Groups) != 0 {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestFormatJSONCleanupOutranksDrift(t *testing.T) {
	result := ConfigResult{Groups: []GroupResult{{
		Name:   "api",
		Status: GroupDrift,
		Drifts: []Drift{{Kind: "modified", Path: "out.txt"}},
	}}}
	result.noteCleanup(errors.New("git worktree remove: boom"))
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	doc := decodeJSON(t, mustFormatJSON(t, RunResult{Configs: []ConfigRun{{
		Path:   "genguard.yaml",
		Result: result,
	}}}))
	if doc.Exit != 2 || doc.Configs[0].Exit != 2 || doc.Configs[0].Error != "git worktree remove: boom" {
		t.Fatalf("doc = %+v", doc)
	}
	if doc.Configs[0].Groups[0].Status != "drift" {
		t.Fatalf("group = %+v", doc.Configs[0].Groups[0])
	}
}

func TestFormatJSONCleanupError(t *testing.T) {
	result := ConfigResult{Groups: []GroupResult{{Name: "api", Status: GroupOK}}}
	result.noteCleanup(errors.New("git worktree remove: boom"))
	doc := decodeJSON(t, mustFormatJSON(t, RunResult{Configs: []ConfigRun{{
		Path:   "genguard.yaml",
		Result: result,
	}}}))
	if doc.Exit != 2 || doc.Configs[0].Exit != 2 || doc.Configs[0].Error != "git worktree remove: boom" {
		t.Fatalf("doc = %+v", doc)
	}
	if doc.Configs[0].Groups[0].Status != "ok" {
		t.Fatalf("group = %+v", doc.Configs[0].Groups[0])
	}
}

func TestFormatJSONTools(t *testing.T) {
	run := RunResult{Configs: []ConfigRun{{
		Path: "genguard.yaml",
		Result: ConfigResult{Groups: []GroupResult{
			{
				Name:   "protobuf",
				Status: GroupDrift,
				Drifts: []Drift{{Kind: "modified", Path: "gen/a.go"}},
				Tools: []ToolResult{
					{Name: "buf", Want: "1.32.0", Have: "1.28.1"},
					{Name: "sqlc", Have: "1.27.0"},
					{Name: "protoc", Want: "1.0.0"},
				},
			},
			{Name: "quiet", Status: GroupSkipped, Tools: []ToolResult{{Name: "buf", Have: "1.0.0"}}},
			{Name: "plain", Status: GroupOK},
		}},
	}}}
	text := mustFormatJSON(t, run)
	if strings.Contains(text, `"tools": null`) {
		t.Fatalf("json = %s", text)
	}
	doc := decodeJSON(t, text)
	got := doc.Configs[0].Groups[0].Tools
	if len(got) != 3 || got[0].Name != "buf" || got[0].Want != "1.32.0" || got[0].Have != "1.28.1" {
		t.Fatalf("tools = %+v", got)
	}
	if got[1].Name != "sqlc" || got[1].Want != "" || got[1].Have != "1.27.0" {
		t.Fatalf("unpinned = %+v", got[1])
	}
	if got[2].Name != "protoc" || got[2].Want != "1.0.0" || got[2].Have != "" {
		t.Fatalf("missing version = %+v", got[2])
	}
	sqlc := text[strings.Index(text, `"name": "sqlc"`):strings.Index(text, `"name": "protoc"`)]
	if strings.Contains(sqlc, `"want"`) {
		t.Fatalf("unpinned tool has want: %s", sqlc)
	}
	protoc := text[strings.Index(text, `"name": "protoc"`):]
	protoc = protoc[:strings.Index(protoc, "}")]
	if strings.Contains(protoc, `"have"`) {
		t.Fatalf("tool with no version has have: %s", protoc)
	}
	if doc.Configs[0].Groups[1].Tools != nil || doc.Configs[0].Groups[2].Tools != nil {
		t.Fatalf("omitted tools = %+v %+v", doc.Configs[0].Groups[1].Tools, doc.Configs[0].Groups[2].Tools)
	}
}

func TestFormatJSONDoesNotEscapePath(t *testing.T) {
	text := mustFormatJSON(t, RunResult{Configs: []ConfigRun{{
		Path: "a<b>&c.yaml",
		Result: ConfigResult{Groups: []GroupResult{{
			Name:   "g",
			Status: GroupDrift,
			Drifts: []Drift{{Kind: "missing", Path: "gen/<id>.go"}},
		}}},
	}}})
	if strings.Contains(text, `\u003c`) || strings.Contains(text, `\u003e`) || strings.Contains(text, `\u0026`) {
		t.Fatalf("json escaped a path: %s", text)
	}
}

func mustFormatJSON(t *testing.T, run RunResult) string {
	t.Helper()
	text, err := FormatJSON(run)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

type jsonDoc struct {
	Exit    int `json:"exit"`
	Configs []struct {
		Path   string `json:"path"`
		Exit   int    `json:"exit"`
		Error  string `json:"error"`
		Groups []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Error  string `json:"error"`
			Drifts []struct {
				Kind string `json:"kind"`
				Path string `json:"path"`
			} `json:"drifts"`
			Tools []struct {
				Name string `json:"name"`
				Want string `json:"want"`
				Have string `json:"have"`
			} `json:"tools"`
		} `json:"groups"`
	} `json:"configs"`
}

func decodeJSON(t *testing.T, text string) jsonDoc {
	t.Helper()
	var doc jsonDoc
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", text, err)
	}
	return doc
}
