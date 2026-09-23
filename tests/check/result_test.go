package check_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestCheckRunsAllGroupsAfterCommandFailure(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}},
		{Name: "greeting", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("genguard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(result.Groups))
	}
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("broken status = %q", result.Groups[0].Status)
	}
	if result.Groups[1].Status != check.GroupDrift {
		t.Fatalf("greeting status = %q", result.Groups[1].Status)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode())
	}
	summary := strings.Join(result.SummaryLines(), "\n")
	if !strings.Contains(summary, "broken: error") {
		t.Fatalf("summary = %q", summary)
	}
	if !strings.Contains(summary, "greeting: drift") {
		t.Fatalf("summary = %q", summary)
	}
}

func TestCheckConfigExitCodeDriftOnly(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("genguard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1", result.ExitCode())
	}
}

func TestCheckConfigExitCodeAllOK(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode())
	}
}

func TestSummaryLine(t *testing.T) {
	cases := []struct {
		name string
		g    check.GroupResult
		want string
	}{
		{
			name: "ok",
			g:    check.GroupResult{Name: "greeting", Status: check.GroupOK},
			want: "  greeting: OK",
		},
		{
			name: "skipped",
			g:    check.GroupResult{Name: "protobuf", Status: check.GroupSkipped},
			want: "  protobuf: skipped",
		},
		{
			name: "one modified",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupDrift,
				Drifts: []check.Drift{{Kind: "modified", Path: "a"}},
			},
			want: "  greeting: drift (1 modified)",
		},
		{
			name: "mixed kinds",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupDrift,
				Drifts: []check.Drift{
					{Kind: "modified", Path: "a"},
					{Kind: "modified", Path: "b"},
					{Kind: "untracked", Path: "c"},
				},
			},
			want: "  greeting: drift (2 modified, 1 untracked)",
		},
		{
			name: "one missing",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupDrift,
				Drifts: []check.Drift{{Kind: "missing", Path: "a"}},
			},
			want: "  greeting: drift (1 missing)",
		},
		{
			name: "unknown kind kept",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupDrift,
				Drifts: []check.Drift{
					{Kind: "modified", Path: "a"},
					{Kind: "other", Path: "b"},
				},
			},
			want: "  greeting: drift (1 modified, 1 other)",
		},
		{
			name: "blank kind singular",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupDrift,
				Drifts: []check.Drift{{Kind: "", Path: "a"}},
			},
			want: "  greeting: drift (1 file)",
		},
		{
			name: "blank kind plural",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupDrift,
				Drifts: []check.Drift{{Kind: "", Path: "a"}, {Kind: "", Path: "b"}},
			},
			want: "  greeting: drift (2 files)",
		},
		{
			name: "blank kind after known kinds",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupDrift,
				Drifts: []check.Drift{
					{Kind: "modified", Path: "a"},
					{Kind: "", Path: "b"},
				},
			},
			want: "  greeting: drift (1 modified, 1 file)",
		},
		{
			name: "zero files",
			g:    check.GroupResult{Name: "greeting", Status: check.GroupDrift},
			want: "  greeting: drift (0 files)",
		},
		{
			name: "flattens multiline error",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupError,
				Err:    errors.New("command failed (exit 1):\nline1\n  line2"),
			},
			want: "  greeting: error (command failed (exit 1): line1 line2)",
		},
		{
			name: "error with drift",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupError,
				Err:    errors.New("command failed (exit 1): boom"),
				Drifts: []check.Drift{{Kind: "modified", Path: "a"}},
			},
			want: "  greeting: error (command failed (exit 1): boom); drift (1 modified)",
		},
		{
			name: "error with mixed drift",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupError,
				Err:    errors.New("command failed (exit 1): boom"),
				Drifts: []check.Drift{
					{Kind: "modified", Path: "a"},
					{Kind: "missing", Path: "b"},
				},
			},
			want: "  greeting: error (command failed (exit 1): boom); drift (1 modified, 1 missing)",
		},
		{
			name: "nil error",
			g:    check.GroupResult{Name: "greeting", Status: check.GroupError},
			want: "  greeting: error (unknown error)",
		},
		{
			name: "whitespace error",
			g: check.GroupResult{
				Name:   "greeting",
				Status: check.GroupError,
				Err:    errors.New(" \n\t "),
			},
			want: "  greeting: error (unknown error)",
		},
		{
			name: "unknown status",
			g:    check.GroupResult{Name: "greeting", Status: "nope"},
			want: "  greeting: unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.g.SummaryLine()
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if strings.Contains(got, "\n") {
				t.Fatalf("summary contains newline: %q", got)
			}
		})
	}
}

func TestSummaryLinesCount(t *testing.T) {
	result := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "one", Status: check.GroupOK},
		{Name: "two", Status: check.GroupDrift, Drifts: []check.Drift{{Kind: "modified", Path: "a"}}},
	}}
	lines := result.SummaryLines()
	if len(lines) != 3 {
		t.Fatalf("lines = %v", lines)
	}
	if lines[0] != "  one: OK" || lines[1] != "  two: drift (1 modified)" {
		t.Fatalf("lines = %v", lines)
	}
	if lines[2] != "2 groups: 1 ok, 1 drift, 0 error" {
		t.Fatalf("totals = %q", lines[2])
	}

	one := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "openapi", Status: check.GroupDrift, Drifts: []check.Drift{
			{Kind: "modified", Path: "generated/models.py"},
		}},
	}}
	lines = one.SummaryLines()
	if lines[len(lines)-1] != "1 group: 0 ok, 1 drift, 0 error" {
		t.Fatalf("totals = %q", lines[len(lines)-1])
	}

	skipped := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "sqlc", Status: check.GroupOK},
		{Name: "protobuf", Status: check.GroupSkipped},
	}}
	lines = skipped.SummaryLines()
	if lines[len(lines)-1] != "2 groups: 1 ok, 0 drift, 0 error, 1 skipped" {
		t.Fatalf("totals = %q", lines[len(lines)-1])
	}
}

func TestAllDriftsConcatenatesGroups(t *testing.T) {
	result := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "a", Status: check.GroupDrift, Drifts: []check.Drift{{Group: "a", Path: "one", Kind: "modified"}}},
		{Name: "b", Status: check.GroupOK},
		{Name: "c", Status: check.GroupDrift, Drifts: []check.Drift{
			{Group: "c", Path: "two", Kind: "missing"},
			{Group: "c", Path: "three", Kind: "untracked"},
		}},
	}}
	got := result.AllDrifts()
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %v", len(got), got)
	}
	if got[0].Path != "one" || got[1].Path != "two" || got[2].Path != "three" {
		t.Fatalf("paths = %v", got)
	}
}

func TestExitCodeUnknownStatusIsError(t *testing.T) {
	result := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "x", Status: "nope"},
		{Name: "y", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "f"}}},
	}}
	if result.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode())
	}
	if line := result.FinalErrorLine(); line != "error: 1 group failed; 1 group drifted" {
		t.Fatalf("final = %q", line)
	}
}

func TestFinalErrorLine(t *testing.T) {
	cases := []struct {
		name   string
		groups []check.GroupResult
		want   string
	}{
		{
			name: "single error keeps detail",
			groups: []check.GroupResult{
				{Name: "g", Status: check.GroupError, Err: errors.New("command failed (exit 3): no output")},
			},
			want: "error: command failed (exit 3): no output",
		},
		{
			name: "error with its own drift stays the command error",
			groups: []check.GroupResult{
				{
					Name:   "g",
					Status: check.GroupError,
					Err:    errors.New("command failed (exit 1): boom"),
					Drifts: []check.Drift{{Kind: "modified", Path: "a"}},
				},
			},
			want: "error: command failed (exit 1): boom",
		},
		{
			name: "single error nil",
			groups: []check.GroupResult{
				{Name: "g", Status: check.GroupError},
			},
			want: "error: 1 group failed",
		},
		{
			name: "unknown status",
			groups: []check.GroupResult{
				{Name: "g", Status: "nope"},
			},
			want: "error: 1 group failed",
		},
		{
			name: "mixed",
			groups: []check.GroupResult{
				{Name: "a", Status: check.GroupError, Err: errors.New("x")},
				{Name: "b", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "f"}}},
			},
			want: "error: 1 group failed; 1 group drifted",
		},
		{
			name: "mixed plurals",
			groups: []check.GroupResult{
				{Name: "a", Status: check.GroupError, Err: errors.New("x")},
				{Name: "b", Status: check.GroupError, Err: errors.New("y")},
				{Name: "c", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "f"}}},
				{Name: "d", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "g"}}},
			},
			want: "error: 2 groups failed; 2 groups drifted",
		},
		{
			name: "two errors",
			groups: []check.GroupResult{
				{Name: "a", Status: check.GroupError, Err: errors.New("x")},
				{Name: "b", Status: check.GroupError, Err: errors.New("y")},
			},
			want: "error: 2 groups failed",
		},
		{
			name: "one drift file",
			groups: []check.GroupResult{
				{Name: "a", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "f"}}},
			},
			want: "error: 1 generated path drifted; commit the generator output or fix the command",
		},
		{
			name: "two drift files",
			groups: []check.GroupResult{
				{Name: "a", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "f"}, {Path: "g"}}},
			},
			want: "error: 2 generated paths drifted; commit the generator output or fix the command",
		},
		{
			name: "ok",
			groups: []check.GroupResult{
				{Name: "a", Status: check.GroupOK},
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := check.ConfigResult{Groups: tc.groups}.FinalErrorLine()
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
