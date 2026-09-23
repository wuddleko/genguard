package check_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestIsolatedDirtyOutputLeavesUserTree(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	dirty := filepath.Join(root, "generated", "hello.txt")
	if err := os.WriteFile(dirty, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, groups = %+v", result.ExitCode(), result.Groups)
	}
	got, err := os.ReadFile(dirty)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "dirty\n" {
		t.Fatalf("user output = %q, isolated check wrote the checkout", got)
	}
}

func TestIsolatedCleanDeletesOnlyInWorktree(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: `python3 -c "import os,sys; sys.exit(0 if not os.path.exists('generated/hello.txt') else 1)"`,
		Outputs: []string{"generated/hello.txt"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	dirty := filepath.Join(root, "generated", "hello.txt")
	if err := os.WriteFile(dirty, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 1 {
		t.Fatalf("exit = %d, groups = %+v", result.ExitCode(), result.Groups)
	}
	if len(result.Groups) != 1 || len(result.Groups[0].Drifts) != 1 {
		t.Fatalf("groups = %+v", result.Groups)
	}
	item := result.Groups[0].Drifts[0]
	if item.Kind != "missing" || item.Path != "generated/hello.txt" {
		t.Fatalf("drift = %+v, want missing generated/hello.txt", item)
	}
	got, err := os.ReadFile(dirty)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "dirty\n" {
		t.Fatalf("user output = %q, clean ran in the checkout", got)
	}
}

func TestIsolatedDriftIsConfigRelative(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	commitNameChange(t, root)
	userOutput := filepath.Join(root, "generated", "hello.txt")
	if err := os.WriteFile(userOutput, []byte("USERDIRT\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 1 {
		t.Fatalf("exit = %d, want 1; groups = %+v", result.ExitCode(), result.Groups)
	}
	if len(result.Groups) != 1 || len(result.Groups[0].Drifts) != 1 {
		t.Fatalf("groups = %+v", result.Groups)
	}
	item := result.Groups[0].Drifts[0]
	if item.Kind != "modified" || item.Path != "generated/hello.txt" || item.Group != "greeting" {
		t.Fatalf("drift = %+v, want config-relative generated/hello.txt", item)
	}
	got, err := os.ReadFile(userOutput)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "USERDIRT\n" {
		t.Fatalf("user output = %q, generator wrote the checkout", got)
	}
	report, err := check.FormatFailureReport(result, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "+hello genguard") || strings.Contains(report, "USERDIRT") {
		t.Fatalf("report =\n%s", report)
	}
}

func TestIsolatedSinceSkipsUnstagedInput(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "queries", "q.sql"), []byte("select 9;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "base")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, result, "sqlc", check.GroupSkipped)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d", result.ExitCode())
	}
	if markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") || markerExists(root, "plain-ran") {
		t.Fatal("isolated check wrote a marker into the user tree")
	}
}

func TestIsolatedSinceRunsCommittedInput(t *testing.T) {
	root := writeSinceRepo(t)
	commitPath(t, root, "queries/q.sql", "select 2;\n")

	result, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "base")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d", result.ExitCode())
	}
	if markerExists(root, "sqlc-ran") || markerExists(root, "plain-ran") {
		t.Fatal("isolated command ran in the user tree")
	}
}

func TestIsolatedAddFailureRunsNoCommand(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "out.txt", `python3 -c "open('ran','w').close()"`, "", nil); err != nil {
		t.Fatal(err)
	}

	_, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "")
	var ge *check.GenguardError
	if !errors.As(err, &ge) {
		t.Fatalf("err = %v, want GenguardError", err)
	}
	if !strings.Contains(err.Error(), "git worktree add:") {
		t.Fatalf("err = %q", err)
	}
	if markerExists(root, "ran") {
		t.Fatal("command ran after isolate failed")
	}
}

func TestIsolatedLoadsHeadConfigNotDirtyCopy(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", `python3 -c "open('ran','w').close()"`, "", nil); err != nil {
		t.Fatal(err)
	}

	result, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, groups = %+v", result.ExitCode(), result.Groups)
	}
	if markerExists(root, "ran") {
		t.Fatal("dirty config command ran")
	}
}

func TestIsolatedMissingConfigIsError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tracked.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "tracked.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "out.txt", "true", "", nil); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(root, "genguard.yaml")
	_, err := check.CheckSinceIsolated(cfgPath, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), cfgPath) || strings.Contains(err.Error(), "genguard-") {
		t.Fatalf("err = %q, want the caller path", err)
	}
	if _, err := config.LoadConfig(cfgPath); err != nil {
		t.Fatal(err)
	}
}

func TestIsolatedParseErrorUsesCallerPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "genguard.yaml")
	if err := os.WriteFile(cfgPath, []byte("groups: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "bad"); err != nil {
		t.Fatal(err)
	}

	_, err := check.CheckSinceIsolated(cfgPath, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), cfgPath) || strings.Contains(err.Error(), "genguard-") {
		t.Fatalf("err = %q, want the caller path", err)
	}
}

func TestIsolatedSinceResolvesCallerUpstream(t *testing.T) {
	root := writeSinceRepo(t)
	commitPath(t, root, "queries/q.sql", "select 2;\n")
	if err := testutil.Git(root, "branch", "-u", "base"); err != nil {
		t.Fatal(err)
	}

	result, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "@{u}")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d", result.ExitCode())
	}
	if markerExists(root, "sqlc-ran") || markerExists(root, "plain-ran") {
		t.Fatal("isolated command ran in the user tree")
	}
}

func TestIsolatedIgnoresCheckoutHook(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	commitNameChange(t, root)
	marker := filepath.Join(root, "hooked.txt")
	hookDir := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\necho hooked >> %q\npython3 scripts/gen.py\n", marker)
	if err := os.WriteFile(filepath.Join(hookDir, "post-checkout"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := check.CheckSinceIsolated(filepath.Join(root, "genguard.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 1 {
		t.Fatalf("exit = %d, groups = %+v", result.ExitCode(), result.Groups)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("checkout hook ran: %v", err)
	}
}
