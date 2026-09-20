package check_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/testutil"
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
