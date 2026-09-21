package check_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
)

func TestConfigRunExitCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		run  check.ConfigRun
		want int
	}{
		{
			name: "setup error wins over empty result",
			run:  check.ConfigRun{Err: errors.New("parse failed")},
			want: 2,
		},
		{
			name: "group error",
			run: check.ConfigRun{Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "g", Status: check.GroupError, Err: errors.New("x")},
			}}},
			want: 2,
		},
		{
			name: "drift",
			run: check.ConfigRun{Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "g", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "a"}}},
			}}},
			want: 1,
		},
		{
			name: "ok",
			run: check.ConfigRun{Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "g", Status: check.GroupOK},
			}}},
			want: 0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.run.ExitCode(); got != tc.want {
				t.Fatalf("ExitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRunResultExitCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		run  check.RunResult
		want int
	}{
		{name: "empty", run: check.RunResult{}, want: 0},
		{
			name: "all ok",
			run: check.RunResult{Configs: []check.ConfigRun{
				{Path: "a", Result: check.ConfigResult{Groups: []check.GroupResult{{Status: check.GroupOK}}}},
				{Path: "b", Result: check.ConfigResult{Groups: []check.GroupResult{{Status: check.GroupOK}}}},
			}},
			want: 0,
		},
		{
			name: "drift among ok",
			run: check.RunResult{Configs: []check.ConfigRun{
				{Path: "a", Result: check.ConfigResult{Groups: []check.GroupResult{{Status: check.GroupOK}}}},
				{Path: "b", Result: check.ConfigResult{Groups: []check.GroupResult{
					{Status: check.GroupDrift, Drifts: []check.Drift{{Path: "x"}}},
				}}},
			}},
			want: 1,
		},
		{
			name: "error beats drift",
			run: check.RunResult{Configs: []check.ConfigRun{
				{Path: "a", Result: check.ConfigResult{Groups: []check.GroupResult{
					{Status: check.GroupDrift, Drifts: []check.Drift{{Path: "x"}}},
				}}},
				{Path: "b", Err: errors.New("load")},
			}},
			want: 2,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.run.ExitCode(); got != tc.want {
				t.Fatalf("ExitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRunResultCountsSkipsSetupFailures(t *testing.T) {
	t.Parallel()
	run := check.RunResult{Configs: []check.ConfigRun{
		{Path: "bad.yaml", Err: errors.New("parse failed")},
		{Path: "ok.yaml", Result: check.ConfigResult{Groups: []check.GroupResult{
			{Name: "one", Status: check.GroupOK},
			{Name: "two", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "x"}}},
			{Name: "three", Status: check.GroupError, Err: errors.New("x")},
		}}},
	}}
	configs, ok, drift, errorsN := run.Counts()
	if configs != 2 || ok != 1 || drift != 1 || errorsN != 1 {
		t.Fatalf("Counts = %d, %d, %d, %d", configs, ok, drift, errorsN)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("ExitCode = %d, want 2", run.ExitCode())
	}
}

func TestRunResultSummaryLines(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(string(filepath.Separator), "repo")
	api := filepath.Join(repo, "api", "genguard.yaml")
	orphan := filepath.Join(string(filepath.Separator), "tmp", "orphan", "genguard.yaml")
	run := check.RunResult{
		RepoRoot: repo,
		Configs: []check.ConfigRun{
			{Path: api, Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "api", Status: check.GroupDrift, Drifts: []check.Drift{{Kind: "modified", Path: "out.txt"}}},
			}}},
			{Path: orphan, Err: errors.New("parse failed")},
		},
	}
	got := strings.Join(run.SummaryLines(), "\n")
	want := strings.Join([]string{
		filepath.Join("api", "genguard.yaml"),
		"  api: drift (1 modified)",
		"1 group: 0 ok, 1 drift, 0 error",
		"",
		orphan,
		"  error (parse failed)",
		"",
		"2 configs: 0 ok, 1 drift, 1 error",
	}, "\n")
	if got != want {
		t.Fatalf("SummaryLines =\n%s\nwant\n%s", got, want)
	}
	configs, ok, drift, errorsN := run.Counts()
	if configs != 2 || ok != 0 || drift != 1 || errorsN != 0 {
		t.Fatalf("Counts = %d, %d, %d, %d", configs, ok, drift, errorsN)
	}
}

func TestRunResultSummaryLinesEmpty(t *testing.T) {
	t.Parallel()
	got := strings.Join(check.RunResult{}.SummaryLines(), "\n")
	if got != "0 configs: 0 ok, 0 drift, 0 error" {
		t.Fatalf("SummaryLines = %q", got)
	}
}

func TestRunResultSummaryLinesSeparatesTotal(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(string(filepath.Separator), "repo")
	run := check.RunResult{RepoRoot: repo, Configs: []check.ConfigRun{{
		Path: filepath.Join(repo, "genguard.yaml"),
		Result: check.ConfigResult{Groups: []check.GroupResult{
			{Name: "g", Status: check.GroupOK},
		}},
	}}}
	got := strings.Join(run.SummaryLines(), "\n")
	want := strings.Join([]string{
		"genguard.yaml",
		"  g: OK",
		"1 group: 1 ok, 0 drift, 0 error",
		"",
		"1 config: 1 ok, 0 drift, 0 error",
	}, "\n")
	if got != want {
		t.Fatalf("SummaryLines =\n%s\nwant\n%s", got, want)
	}
}

func TestRunResultSummaryLinesRepoRootDisplaysDot(t *testing.T) {
	t.Parallel()
	repo := filepath.Join(string(filepath.Separator), "repo")
	run := check.RunResult{RepoRoot: repo, Configs: []check.ConfigRun{{
		Path: repo,
		Err:  errors.New("parse\nfailed\there"),
	}}}
	lines := run.SummaryLines()
	if lines[0] != "." || lines[1] != "  error (parse failed here)" {
		t.Fatalf("lines = %q", lines)
	}
}

func TestRunResultSummaryLinesMixedGroupIsConfigError(t *testing.T) {
	t.Parallel()
	run := check.RunResult{Configs: []check.ConfigRun{{
		Path: "a.yaml",
		Result: check.ConfigResult{Groups: []check.GroupResult{
			{Name: "a", Status: check.GroupError, Err: errors.New("x")},
			{Name: "b", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "f", Kind: "modified"}}},
		}},
	}}}
	got := strings.Join(run.SummaryLines(), "\n")
	if !strings.Contains(got, "1 config: 0 ok, 0 drift, 1 error") {
		t.Fatalf("SummaryLines =\n%s", got)
	}
}

func TestRunResultFinalErrorLine(t *testing.T) {
	t.Parallel()
	drift := func(paths ...string) check.ConfigRun {
		drifts := make([]check.Drift, len(paths))
		for i, path := range paths {
			drifts[i] = check.Drift{Path: path, Kind: "modified"}
		}
		return check.ConfigRun{Result: check.ConfigResult{Groups: []check.GroupResult{
			{Name: "g", Status: check.GroupDrift, Drifts: drifts},
		}}}
	}
	groupErr := check.ConfigRun{Result: check.ConfigResult{Groups: []check.GroupResult{
		{Name: "g", Status: check.GroupError, Err: errors.New("command failed (exit 3): no output")},
	}}}
	ok := check.ConfigRun{Result: check.ConfigResult{Groups: []check.GroupResult{
		{Name: "g", Status: check.GroupOK},
	}}}
	setup := check.ConfigRun{Err: errors.New("parse failed")}

	cases := []struct {
		name string
		run  check.RunResult
		want string
	}{
		{name: "empty", run: check.RunResult{}, want: ""},
		{name: "ok", run: check.RunResult{Configs: []check.ConfigRun{ok}}, want: ""},
		{
			name: "single drift keeps path detail",
			run:  check.RunResult{Configs: []check.ConfigRun{drift("a")}},
			want: "error: 1 generated path drifted; commit the generator output or fix the command",
		},
		{
			name: "two drifting configs sum paths",
			run:  check.RunResult{Configs: []check.ConfigRun{drift("a"), drift("b", "c")}},
			want: "error: 3 generated paths drifted; commit the generator output or fix the command",
		},
		{
			name: "single setup error keeps detail",
			run:  check.RunResult{Configs: []check.ConfigRun{setup, ok}},
			want: "error: parse failed",
		},
		{
			name: "single group error keeps detail",
			run:  check.RunResult{Configs: []check.ConfigRun{groupErr}},
			want: "error: command failed (exit 3): no output",
		},
		{
			name: "two failures",
			run:  check.RunResult{Configs: []check.ConfigRun{setup, groupErr}},
			want: "error: 2 configs failed",
		},
		{
			name: "error and drift",
			run:  check.RunResult{Configs: []check.ConfigRun{setup, drift("a")}},
			want: "error: 1 config failed; 1 config drifted",
		},
		{
			name: "one failure and two drifts",
			run:  check.RunResult{Configs: []check.ConfigRun{setup, drift("a"), drift("b")}},
			want: "error: 1 config failed; 2 configs drifted",
		},
		{
			name: "mixed groups stay on the config line",
			run: check.RunResult{Configs: []check.ConfigRun{{
				Result: check.ConfigResult{Groups: []check.GroupResult{
					{Name: "a", Status: check.GroupError, Err: errors.New("x")},
					{Name: "b", Status: check.GroupDrift, Drifts: []check.Drift{{Path: "f"}}},
				}},
			}}},
			want: "error: 1 group failed; 1 group drifted",
		},
		{
			name: "single config with two paths",
			run:  check.RunResult{Configs: []check.ConfigRun{drift("a", "b")}},
			want: "error: 2 generated paths drifted; commit the generator output or fix the command",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.run.FinalErrorLine(); got != tc.want {
				t.Fatalf("FinalErrorLine = %q, want %q", got, tc.want)
			}
		})
	}
}
