package check

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
					{Kind: "modified", Path: "gen/a.go"},
					{Kind: "missing", Path: "gen/a,b.go"},
				}},
			}},
		}},
	}
	text := FormatAnnotations(run)
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
	result.noteCleanup(errors.New("remove failed"))
	text := FormatAnnotations(RunResult{
		RepoRoot: root,
		Configs: []ConfigRun{
			{Path: filepath.Join(root, "genguard.yaml"), Result: result},
			{Path: filepath.Join(root, "api", "genguard.yaml"), Err: errors.New("parse failed")},
		},
	})
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
	})
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
				Drifts: []Drift{{Kind: "modified", Path: "gen/a.go"}},
			}}},
		}},
	}

	t.Setenv("GITHUB_WORKSPACE", parent)
	text := FormatAnnotations(run)
	want := "::error file=src/api/gen/a.go,title=protobuf::modified\n"
	if text != want {
		t.Fatalf("annotations = %q", text)
	}

	t.Setenv("GITHUB_WORKSPACE", root)
	text = FormatAnnotations(run)
	want = "::error file=api/gen/a.go,title=protobuf::modified\n"
	if text != want {
		t.Fatalf("repo root annotations = %q", text)
	}

	t.Setenv("GITHUB_WORKSPACE", filepath.Join(parent, "other"))
	text = FormatAnnotations(run)
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
	}}})
	if text != "" {
		t.Fatalf("annotations = %q", text)
	}
}
