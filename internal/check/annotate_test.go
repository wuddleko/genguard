package check

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/actions"
)

func TestFormatAnnotationsDriftFile(t *testing.T) {
	root := t.TempDir()
	run := RunResult{
		RepoRoot: root,
		Configs: []ConfigRun{{
			Path: filepath.Join(root, "api", "genguard.yaml"),
			Result: ConfigResult{Groups: []GroupResult{
				{Name: "plain", Status: GroupOK},
				{Name: "protobuf", Status: GroupDrift, Drifts: []Drift{
					{Kind: "modified", Path: "api/gen/a.go"},
					{Kind: "missing", Path: "api/gen/a,b.go"},
				}},
			}},
		}},
	}
	text := FormatAnnotations(run, actions.Env{})
	want := "::error file=api/gen/a.go,title=protobuf::modified\n" +
		"::error file=api/gen/a%2Cb.go,title=protobuf::missing\n"
	if text != want {
		t.Fatalf("annotations = %q", text)
	}
}

func TestFormatAnnotationsErrorAndCleanup(t *testing.T) {
	root := t.TempDir()
	result := ConfigResult{Groups: []GroupResult{{
		Name:   "protobuf",
		Status: GroupError,
		Err:    errors.New("command failed (exit 1): line1\nline2"),
		Drifts: []Drift{{Kind: "modified", Path: "gen/a.go"}},
	}}}
	result.note(errors.New("remove failed"))
	text := FormatAnnotations(RunResult{
		RepoRoot: root,
		Configs: []ConfigRun{
			{Path: filepath.Join(root, "genguard.yaml"), Result: result},
			{Path: filepath.Join(root, "api", "genguard.yaml"), Err: errors.New("parse failed")},
		},
	}, actions.Env{})
	for _, line := range []string{
		"::error file=genguard.yaml,title=protobuf::command failed (exit 1): line1 line2",
		"::error file=gen/a.go,title=protobuf::modified",
		"::error file=genguard.yaml::remove failed",
		"::error file=api/genguard.yaml::parse failed",
	} {
		if !strings.Contains(text, line) {
			t.Fatalf("missing %q in %q", line, text)
		}
	}
}

func TestFormatAnnotationsEscapesPropertiesOnly(t *testing.T) {
	root := t.TempDir()
	text := FormatAnnotations(RunResult{
		RepoRoot: root,
		Configs: []ConfigRun{{
			Path: filepath.Join(root, "genguard.yaml"),
			Result: ConfigResult{Groups: []GroupResult{{
				Name:   "a:b,c",
				Status: GroupError,
				Err:    errors.New("100% failed: no,pe"),
			}}},
		}},
	}, actions.Env{})
	want := "::error file=genguard.yaml,title=a%3Ab%2Cc::100%25 failed: no,pe\n"
	if text != want {
		t.Fatalf("annotations = %q", text)
	}
}

func TestFormatAnnotationsWorkspacePrefix(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "src")
	if err := os.MkdirAll(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := RunResult{
		RepoRoot: root,
		Configs: []ConfigRun{{
			Path: filepath.Join(root, "api", "genguard.yaml"),
			Result: ConfigResult{Groups: []GroupResult{{
				Name:   "protobuf",
				Status: GroupDrift,
				Drifts: []Drift{{Kind: "modified", Path: "api/gen/a.go"}},
			}}},
		}},
	}

	text := FormatAnnotations(run, actions.Env{Workspace: parent})
	want := "::error file=src/api/gen/a.go,title=protobuf::modified\n"
	if text != want {
		t.Fatalf("annotations = %q", text)
	}

	text = FormatAnnotations(run, actions.Env{Workspace: root})
	want = "::error file=api/gen/a.go,title=protobuf::modified\n"
	if text != want {
		t.Fatalf("repo root annotations = %q", text)
	}

	text = FormatAnnotations(run, actions.Env{Workspace: filepath.Join(parent, "other")})
	if text != want {
		t.Fatalf("outside workspace annotations = %q", text)
	}
}

func TestFormatAnnotationsSkipsOK(t *testing.T) {
	text := FormatAnnotations(RunResult{Configs: []ConfigRun{{
		Path: "genguard.yaml",
		Result: ConfigResult{Groups: []GroupResult{
			{Name: "sqlc", Status: GroupSkipped},
			{Name: "plain", Status: GroupOK},
		}},
	}}}, actions.Env{})
	if text != "" {
		t.Fatalf("annotations = %q", text)
	}
}
