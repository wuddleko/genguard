package check_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestFormatFailureReportSections(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "hello.txt"), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "openapi", Status: check.GroupOK},
		{Name: "sqlc", Status: check.GroupDrift, Drifts: []check.Drift{
			{Group: "sqlc", Kind: "modified", Path: "generated/hello.txt"},
		}},
	}}
	report, err := check.FormatFailureReport(result, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(report, "Summary\n") {
		t.Fatalf("report = %q", report)
	}
	if strings.Contains(report, "Could not access 'HEAD'") {
		t.Fatalf("report used a non-repo root: %q", report)
	}

	summary := strings.Index(report, "  sqlc: drift (1 modified)")
	drift := strings.Index(report, "\nDrift\n")
	item := strings.Index(report, "[modified] sqlc: generated/hello.txt")
	diff := strings.Index(report, "diff --git")
	final := strings.Index(report, "error: 1 generated path drifted")
	if summary < 0 || drift < 0 || item < 0 || diff < 0 || final < 0 {
		t.Fatalf("report = %q", report)
	}
	if !(summary < drift && drift < item && item < diff && diff < final) {
		t.Fatalf("section order: %q", report)
	}
}

func TestFormatFailureReportKeepsSectionsWhenDiffFails(t *testing.T) {
	result := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "sqlc", Status: check.GroupDrift, Drifts: []check.Drift{
			{Group: "sqlc", Kind: "modified", Path: "generated/hello.txt"},
		}},
	}}
	report, err := check.FormatFailureReport(result, filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatalf("expected diff error, report = %q", report)
	}
	if !strings.Contains(report, "Summary\n") {
		t.Fatalf("report = %q", report)
	}
	if !strings.Contains(report, "  sqlc: drift (1 modified)") {
		t.Fatalf("report = %q", report)
	}
	if !strings.Contains(report, "\nDrift\n") {
		t.Fatalf("report = %q", report)
	}
	if !strings.Contains(report, "[modified] sqlc: generated/hello.txt") {
		t.Fatalf("report = %q", report)
	}
	if strings.Contains(report, "error: 1 generated path drifted") {
		t.Fatalf("unexpected final error line: %q", report)
	}
}

func TestFormatFailureReportCommandTails(t *testing.T) {
	result := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "greeting", Status: check.GroupError, Err: errors.New("command failed (exit 3)"), CommandTail: "line1\nline2\n"},
		{Name: "quiet", Status: check.GroupError, Err: errors.New("command failed (exit 1): no output")},
		{Name: "other", Status: check.GroupError, Err: errors.New("command failed (exit 4)"), CommandTail: "boom"},
	}}
	report, err := check.FormatFailureReport(result, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const want = "greeting:\n" +
		"line1\n" +
		"line2\n" +
		"\n" +
		"other:\n" +
		"boom\n" +
		"\n" +
		"Summary\n" +
		"  greeting: error (command failed (exit 3))\n" +
		"  quiet: error (command failed (exit 1): no output)\n" +
		"  other: error (command failed (exit 4))\n" +
		"3 groups: 0 ok, 0 drift, 3 error\n" +
		"\n" +
		"error: 3 groups failed\n"
	if report != want {
		t.Fatalf("report = %q\nwant %q", report, want)
	}
}

func TestFormatRunFailureReportCommandTails(t *testing.T) {
	root := t.TempDir()
	api := filepath.Join(root, "services", "api", "genguard.yaml")
	web := filepath.Join(root, "services", "web", "genguard.yaml")
	run := check.RunResult{
		RepoRoot: root,
		Configs: []check.ConfigRun{
			{Path: api, Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "greeting", Status: check.GroupError, Err: errors.New("command failed (exit 3)"), CommandTail: "line1\nline2\n"},
				{Name: "quiet", Status: check.GroupError, Err: errors.New("command failed (exit 1): no output")},
			}}},
			{Path: web, Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "assets", Status: check.GroupError, Err: errors.New("command failed (exit 4)"), CommandTail: "boom\n"},
			}}},
		},
	}
	report, err := check.FormatRunFailureReport(run)
	if err != nil {
		t.Fatal(err)
	}
	head := filepath.Join("services", "api", "genguard.yaml") + ": greeting:\nline1\nline2\n\n" +
		filepath.Join("services", "web", "genguard.yaml") + ": assets:\nboom\n\nSummary\n"
	if !strings.HasPrefix(report, head) {
		t.Fatalf("report = %q\nwant prefix %q", report, head)
	}
	if strings.Contains(strings.Split(report, "Summary\n")[0], "quiet:") {
		t.Fatalf("empty tail was printed: %q", report)
	}
}

func TestFormatFailureReportErrorsOnly(t *testing.T) {
	result := check.ConfigResult{Groups: []check.GroupResult{
		{Name: "broken", Status: check.GroupError, Err: errors.New("command failed (exit 3): no output")},
	}}
	report, err := check.FormatFailureReport(result, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(report, "Summary\n") {
		t.Fatalf("report = %q", report)
	}
	if !strings.Contains(report, "  broken: error (command failed (exit 3): no output)") {
		t.Fatalf("report = %q", report)
	}
	if strings.Contains(report, "\nDrift\n") {
		t.Fatalf("unexpected drift section: %q", report)
	}
	if !strings.Contains(report, "error: command failed (exit 3): no output") {
		t.Fatalf("report = %q", report)
	}
}

func TestFormatRunFailureReportSections(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "hello.txt"), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := check.RunResult{
		RepoRoot: root,
		Configs: []check.ConfigRun{
			{Path: filepath.Join(root, "genguard.yaml"), Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "sqlc", Status: check.GroupDrift, Drifts: []check.Drift{
					{Group: "sqlc", Kind: "modified", Path: "generated/hello.txt"},
				}},
			}}},
			{Path: filepath.Join(root, "web", "genguard.yaml"), Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "web", Status: check.GroupOK},
			}}},
		},
	}
	report, err := check.FormatRunFailureReport(run)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(report, "Summary\n") {
		t.Fatalf("report = %q", report)
	}
	summary := strings.Index(report, "genguard.yaml\n  sqlc: drift (1 modified)")
	other := strings.Index(report, filepath.Join("web", "genguard.yaml")+"\n  web: OK")
	totals := strings.Index(report, "2 configs: 1 ok, 1 drift, 0 error")
	drift := strings.Index(report, "\nDrift\n")
	item := strings.Index(report, "[modified] sqlc: generated/hello.txt")
	diff := strings.Index(report, "diff --git")
	final := strings.Index(report, "error: 1 generated path drifted")
	if summary < 0 || other < 0 || totals < 0 || drift < 0 || item < 0 || diff < 0 || final < 0 {
		t.Fatalf("report = %q", report)
	}
	if !(summary < other && other < totals && totals < drift && drift < item && item < diff && diff < final) {
		t.Fatalf("section order: %q", report)
	}
	if strings.Contains(report, "\nDrift\n"+filepath.Join("web", "genguard.yaml")) {
		t.Fatalf("ok config listed under drift: %q", report)
	}
}

func TestFormatRunFailureReportSetupError(t *testing.T) {
	run := check.RunResult{Configs: []check.ConfigRun{
		{Path: "bad.yaml", Err: errors.New("parse failed")},
	}}
	report, err := check.FormatRunFailureReport(run)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(report, "\nDrift\n") {
		t.Fatalf("unexpected drift section: %q", report)
	}
	if !strings.Contains(report, "  error (parse failed)") {
		t.Fatalf("report = %q", report)
	}
	if !strings.Contains(report, "error: parse failed") {
		t.Fatalf("report = %q", report)
	}
}

func TestFormatRunFailureReportKeepsSectionsWhenDiffFails(t *testing.T) {
	run := check.RunResult{Configs: []check.ConfigRun{
		{Path: filepath.Join(t.TempDir(), "missing", "genguard.yaml"), Result: check.ConfigResult{Groups: []check.GroupResult{
			{Name: "sqlc", Status: check.GroupDrift, Drifts: []check.Drift{
				{Group: "sqlc", Kind: "modified", Path: "generated/hello.txt"},
			}},
		}}},
	}}
	report, err := check.FormatRunFailureReport(run)
	if err == nil {
		t.Fatalf("expected diff error, report = %q", report)
	}
	if !strings.Contains(report, "Summary\n") || !strings.Contains(report, "\nDrift\n") {
		t.Fatalf("report = %q", report)
	}
	if !strings.Contains(report, "[modified] sqlc: generated/hello.txt") {
		t.Fatalf("report = %q", report)
	}
	if strings.Contains(report, "error: 1 generated path drifted") {
		t.Fatalf("unexpected final error line: %q", report)
	}
}

func TestFormatRunFailureReportStopsBeforeLaterConfigs(t *testing.T) {
	run := check.RunResult{Configs: []check.ConfigRun{
		{Path: filepath.Join(t.TempDir(), "missing", "genguard.yaml"), Result: check.ConfigResult{Groups: []check.GroupResult{
			{Name: "sqlc", Status: check.GroupDrift, Drifts: []check.Drift{
				{Group: "sqlc", Kind: "modified", Path: "generated/hello.txt"},
			}},
		}}},
		{Path: filepath.Join(t.TempDir(), "later", "genguard.yaml"), Result: check.ConfigResult{Groups: []check.GroupResult{
			{Name: "web", Status: check.GroupDrift, Drifts: []check.Drift{
				{Group: "web", Kind: "modified", Path: "not-reached.txt"},
			}},
		}}},
	}}
	report, err := check.FormatRunFailureReport(run)
	if err == nil {
		t.Fatalf("expected diff error, report = %q", report)
	}
	if !strings.Contains(report, "[modified] sqlc: generated/hello.txt") {
		t.Fatalf("report = %q", report)
	}
	if strings.Contains(report, "not-reached.txt") {
		t.Fatalf("later config was rendered:\n%s", report)
	}
	if strings.Contains(report, "error: ") {
		t.Fatalf("unexpected final error line: %q", report)
	}
}

func TestFormatRunFailureReportKeepsEarlierDiffWhenLaterDiffFails(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "hello.txt"), []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run := check.RunResult{
		RepoRoot: root,
		Configs: []check.ConfigRun{
			{Path: filepath.Join(root, "genguard.yaml"), Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "sqlc", Status: check.GroupDrift, Drifts: []check.Drift{
					{Group: "sqlc", Kind: "modified", Path: "generated/hello.txt"},
				}},
			}}},
			{Path: filepath.Join(t.TempDir(), "gone", "genguard.yaml"), Result: check.ConfigResult{Groups: []check.GroupResult{
				{Name: "web", Status: check.GroupDrift, Drifts: []check.Drift{
					{Group: "web", Kind: "modified", Path: "later-only.txt"},
				}},
			}}},
		},
	}
	report, err := check.FormatRunFailureReport(run)
	if err == nil {
		t.Fatalf("expected diff error, report = %q", report)
	}
	if !strings.Contains(report, "diff --git") {
		t.Fatalf("earlier diff missing:\n%s", report)
	}
	if !strings.Contains(report, "[modified] web: later-only.txt") {
		t.Fatalf("later drift header missing:\n%s", report)
	}
	if strings.Contains(report, "error: ") {
		t.Fatalf("unexpected final error line:\n%s", report)
	}
}
