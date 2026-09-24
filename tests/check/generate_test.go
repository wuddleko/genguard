package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestRunWritesWithoutFailingOnDrift(t *testing.T) {
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
	result, err := check.RunConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
	assertGroupStatus(t, result, "greeting", check.GroupOK)
	got, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello genguard\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunSinceSkipsUnchangedAndDoesNotClean(t *testing.T) {
	root := writeSinceRepo(t)
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.RunSince(cfg, "base")
	if err != nil {
		t.Fatal(err)
	}

	assertGroupStatus(t, result, "sqlc", check.GroupSkipped)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("skipped group ran its command")
	}
	got, err := os.ReadFile(filepath.Join(root, "gen", "a.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package gen\n" {
		t.Fatalf("clean ran on a skipped group: %q", got)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d", result.ExitCode())
	}
}

func TestRunCommandFailureExit2(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "broken",
		Command: "exit 3",
		Outputs: []string{"generated/hello.txt"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.RunConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	assertGroupStatus(t, result, "broken", check.GroupError)
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %+v", result.AllDrifts())
	}
}

func TestRunContinuesAfterCommandFailure(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}},
		{Name: "ok", Command: `python3 -c "open('ran','w').close()"`, Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.RunConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	assertGroupStatus(t, result, "broken", check.GroupError)
	assertGroupStatus(t, result, "ok", check.GroupOK)
	if !markerExists(root, "ran") {
		t.Fatal("later group did not run")
	}
}

func TestRunSinceBlankRefRunsEveryGroup(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.RunSince(cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d", result.ExitCode())
	}
	assertGroupStatus(t, result, "greeting", check.GroupOK)
}

func TestRunBadSinceRef(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = check.RunSince(cfg, "not-a-ref")
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("error = %v", err)
	}
}
