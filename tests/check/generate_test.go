package check_test

import (
	"os"
	"os/exec"
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
	if !markerExists(root, "plain-ran") {
		t.Fatal("group with no inputs did not run")
	}
	got, err := os.ReadFile(filepath.Join(root, "gen", "a.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package gen\n" {
		t.Fatalf("clean ran on a skipped group: %q", got)
	}
	joined := strings.Join(result.SummaryLines(), "\n")
	if !strings.Contains(joined, "  sqlc: skipped") || !strings.Contains(joined, "  protobuf: skipped") {
		t.Fatalf("summary = %q", joined)
	}
	if !strings.Contains(joined, "3 groups: 1 ok, 0 drift, 0 error, 2 skipped") {
		t.Fatalf("summary = %q", joined)
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %+v", result.AllDrifts())
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
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "command failed (exit 3)") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %+v", result.AllDrifts())
	}
	report, err := check.FormatFailureReport(result, root)
	if err != nil {
		t.Fatal(err)
	}
	const want = "Summary\n" +
		"  broken: error (command failed (exit 3): no output)\n" +
		"1 group: 0 ok, 0 drift, 1 error\n" +
		"\n" +
		"error: command failed (exit 3): no output\n"
	if report != want {
		t.Fatalf("report = %q, want %q", report, want)
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
	root := writeSinceRepo(t)
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = check.RunSince(cfg, "not-a-ref")
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("error = %v", err)
	}
	if markerExists(root, "plain-ran") || markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("bad ref ran a group")
	}
}

func TestRunWithoutSinceRunsEveryGroup(t *testing.T) {
	t.Run("config", func(t *testing.T) {
		root := writeSinceRepo(t)
		assertRunEveryGroup(t, root, mustRunConfig(t, root))
	})
	for _, since := range []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "space", value: " "},
		{name: "tab", value: "\t"},
	} {
		since := since
		t.Run(since.name, func(t *testing.T) {
			root := writeSinceRepo(t)
			assertRunEveryGroup(t, root, mustRunSince(t, root, since.value))
		})
	}
}

func TestRunSinceTrimsRef(t *testing.T) {
	root := writeSinceRepo(t)
	commitPath(t, root, "queries/q.sql", "select 2;\n")

	result := mustRunSince(t, root, "  base  ")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("padded ref did not select only sqlc")
	}
}

func TestRunSinceRunsChangedInput(t *testing.T) {
	root := writeSinceRepo(t)
	commitPath(t, root, "queries/q.sql", "select 2;\n")

	result := mustRunSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %+v", result.AllDrifts())
	}
	if !markerExists(root, "sqlc-ran") || !markerExists(root, "plain-ran") {
		t.Fatal("changed input did not run sqlc")
	}
	if markerExists(root, "proto-ran") {
		t.Fatal("unchanged protobuf ran")
	}
	got, err := os.ReadFile(filepath.Join(root, "gen", "a.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package gen\n" {
		t.Fatalf("clean ran on a skipped group: %q", got)
	}
}

func TestRunSinceRunsStagedInput(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "queries", "q.sql"), []byte("select 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "queries/q.sql"); err != nil {
		t.Fatal(err)
	}

	result := mustRunSince(t, root, "HEAD")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("staged input did not select only sqlc")
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunSinceRunsDeletedInput(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.Remove(filepath.Join(root, "queries", "q.sql")); err != nil {
		t.Fatal(err)
	}

	result := mustRunSince(t, root, "HEAD")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("deleted input did not select only sqlc")
	}
}

func TestRunSinceRunsHandEditedOutput(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "internal", "db", "out.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustRunSince(t, root, "HEAD")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %+v", result.AllDrifts())
	}
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("output edit did not select only sqlc")
	}
	got, err := os.ReadFile(filepath.Join(root, "internal", "db", "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "edited\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunSinceUntrackedInputRuns(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "queries", "new.sql"), []byte("select 3;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := mustRunSince(t, root, "HEAD")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("untracked input did not select only sqlc")
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunSinceIgnoredInputDoesNotRun(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "queries", "skip.ignore"), []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := mustRunSince(t, root, "HEAD")
	assertGroupStatus(t, result, "sqlc", check.GroupSkipped)
	if markerExists(root, "sqlc-ran") {
		t.Fatal("gitignored input ran sqlc")
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunSinceRunsWhenWorktreeMatchesBaseNotHEAD(t *testing.T) {
	root := writeSinceRepo(t)
	commitPath(t, root, "internal/db/out.txt", "edited\n")
	if err := os.WriteFile(filepath.Join(root, "internal/db/out.txt"), []byte("db\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustRunSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("worktree matching base did not select only sqlc")
	}
	got, err := os.ReadFile(filepath.Join(root, "internal/db/out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "db\n" {
		t.Fatalf("output = %q", got)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunSinceRunsInputRestoredToBase(t *testing.T) {
	root := writeSinceRepo(t)
	commitPath(t, root, "queries/q.sql", "select 2;\n")
	if err := os.WriteFile(filepath.Join(root, "queries/q.sql"), []byte("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustRunSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("input restored to base did not select only sqlc")
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunSinceRunsWhenConfigCommandChanges(t *testing.T) {
	root := writeSinceRepo(t)
	groups := sinceGroups()
	groups[0].Command = `python3 -c "open('sqlc-ran','w').close(); open('sqlc-cmd','w').close()"`
	if _, err := testutil.WriteGenguardConfig(root, "", "", "", groups); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "change command"); err != nil {
		t.Fatal(err)
	}

	result := mustRunSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupOK)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if !markerExists(root, "sqlc-cmd") || !markerExists(root, "proto-ran") || !markerExists(root, "plain-ran") {
		t.Fatal("config command change did not run groups with inputs")
	}
	got, err := os.ReadFile(filepath.Join(root, "gen", "a.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package gen\n" {
		t.Fatalf("protobuf output = %q", got)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunSinceRunsUntrackedConfig(t *testing.T) {
	root := writeSinceRepo(t)
	writeSinceFile(t, root, "fresh/in.txt", "in\n")
	writeSinceFile(t, root, "fresh/out.txt", "out\n")
	if err := testutil.Git(root, "add", "fresh/in.txt", "fresh/out.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "fresh files"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "fresh")
	path, err := testutil.WriteGenguardConfig(dir, "", "", "genguard.yml", []testutil.GroupSpec{{
		Name:    "fresh",
		Command: `python3 -c "open('fresh-ran','w').close()"`,
		Inputs:  []string{"in.txt"},
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.RunSince(cfg, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || result.Groups[0].Status != check.GroupOK {
		t.Fatalf("fresh = %+v", result.Groups)
	}
	if !markerExists(dir, "fresh-ran") {
		t.Fatal("untracked config did not run")
	}
	if result.ExitCode() != 0 || len(result.AllDrifts()) != 0 {
		t.Fatalf("exit = %d drifts = %+v", result.ExitCode(), result.AllDrifts())
	}
}

func TestRunSinceRunsMissingLiteralOutput(t *testing.T) {
	root := writeSinceRepo(t)
	groups := sinceGroups()
	groups[0].Outputs = append(append([]string{}, groups[0].Outputs...), "internal/db/missing.txt")
	if _, err := testutil.WriteGenguardConfig(root, "", "", "", groups); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "declare missing output"); err != nil {
		t.Fatal(err)
	}

	result := mustRunSince(t, root, "HEAD")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %+v", result.AllDrifts())
	}
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("missing declared output did not select only sqlc")
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "db", "missing.txt")); !os.IsNotExist(err) {
		t.Fatalf("missing.txt = %v", err)
	}
}

func TestRunSinceRunsDeletionOfOutputAddedAfterBase(t *testing.T) {
	root := writeSinceRepo(t)
	groups := sinceGroups()
	groups[0].Outputs = append(append([]string{}, groups[0].Outputs...), "internal/db/extra.txt")
	if _, err := testutil.WriteGenguardConfig(root, "", "", "", groups); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "declare extra"); err != nil {
		t.Fatal(err)
	}
	// The declaration is in base, so the missing file, not the config, selects the group.
	if err := testutil.Git(root, "branch", "-f", "base"); err != nil {
		t.Fatal(err)
	}
	writeSinceFile(t, root, "internal/db/extra.txt", "extra\n")
	if err := testutil.Git(root, "add", "internal/db/extra.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "extra"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "internal/db/extra.txt")); err != nil {
		t.Fatal(err)
	}

	result := mustRunSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("deleted output added after base did not select only sqlc")
	}
	if _, err := os.Stat(filepath.Join(root, "internal/db/extra.txt")); !os.IsNotExist(err) {
		t.Fatalf("extra.txt = %v", err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunSinceSubdirectoryConfigSelectsItsGroup(t *testing.T) {
	root := writeSinceRepo(t)
	apiConfig := filepath.Join(root, "api", "genguard.yaml")
	cfg, err := config.LoadConfig(apiConfig)
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := check.RunSince(cfg, "base")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, skipped, "api", check.GroupSkipped)
	if markerExists(filepath.Join(root, "api"), "api-ran") || markerExists(root, "sqlc-ran") {
		t.Fatal("unchanged subdirectory config ran")
	}

	commitPath(t, root, "api/src/a.txt", "changed\n")
	result, err := check.RunSince(cfg, "base")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, result, "api", check.GroupOK)
	if !markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("changed subdirectory input did not run")
	}
	if markerExists(root, "sqlc-ran") {
		t.Fatal("root config ran")
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunSinceDirectoryAndGlobOutputsDoNotForceRun(t *testing.T) {
	root := writeSinceRepo(t)
	groups := sinceGroups()
	groups[0].Outputs = []string{"missing-dir/", "missing/*.go"}
	commitConfigAsBase(t, root, groups)

	result := mustRunSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupSkipped)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("directory or glob output forced a run")
	}
	if !markerExists(root, "plain-ran") {
		t.Fatal("group with no inputs did not run")
	}
}

func TestRunSinceCleanWipesSelectedGroupOnly(t *testing.T) {
	root := writeSinceRepo(t)
	groups := sinceGroups()
	groups[1].Command = runProtoCleanProbe
	commitConfigAsBase(t, root, groups)
	commitPath(t, root, "proto/a.proto", "syntax = \"proto3\";\n// changed\n")

	result := mustRunSince(t, root, "base")
	assertGroupStatus(t, result, "protobuf", check.GroupOK)
	assertGroupStatus(t, result, "sqlc", check.GroupSkipped)
	if result.Groups[1].Err != nil {
		t.Fatalf("protobuf err = %v", result.Groups[1].Err)
	}
	if !markerExists(root, "proto-ran") || markerExists(root, "sqlc-ran") {
		t.Fatal("clean did not select only protobuf")
	}
	got, err := os.ReadFile(filepath.Join(root, "gen", "a.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEW\n" {
		t.Fatalf("output = %q", got)
	}
	db, err := os.ReadFile(filepath.Join(root, "internal", "db", "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(db) != "db\n" {
		t.Fatalf("sqlc output = %q", db)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
}

func TestRunCleanRemovesOrphanAndExits0(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "python3 scripts/gen.py",
		Outputs: []string{"generated/"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	commitOrphanGeneratedFile(t, root)

	result := mustRunConfig(t, root)
	assertGroupStatus(t, result, "greeting", check.GroupOK)
	if result.ExitCode() != 0 || len(result.AllDrifts()) != 0 {
		t.Fatalf("exit = %d drifts = %+v", result.ExitCode(), result.AllDrifts())
	}
	if _, err := os.Stat(filepath.Join(root, "generated", "extra.txt")); !os.IsNotExist(err) {
		t.Fatalf("orphan still on disk: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunWithoutCleanLeavesOrphan(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	commitOrphanGeneratedFile(t, root)

	result := mustRunConfig(t, root)
	assertGroupStatus(t, result, "greeting", check.GroupOK)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "extra.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "stale\n" {
		t.Fatalf("orphan = %q", got)
	}
}

func TestRunCleanSuccessLeavesMissingOutputExit0(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "true",
		Outputs: []string{"generated/hello.txt"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustRunConfig(t, root)
	assertGroupStatus(t, result, "greeting", check.GroupOK)
	if result.ExitCode() != 0 || len(result.AllDrifts()) != 0 {
		t.Fatalf("exit = %d drifts = %+v", result.ExitCode(), result.AllDrifts())
	}
	if _, err := os.Stat(filepath.Join(root, "generated", "hello.txt")); !os.IsNotExist(err) {
		t.Fatalf("hello.txt = %v", err)
	}
}

func TestRunCleanCommandFailureAfterWipeContinues(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "greeting", Command: "exit 3", Outputs: []string{"generated/"}, Clean: true},
		{Name: "later", Command: "test ! -f generated/hello.txt", Outputs: []string{"other.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustRunConfig(t, root)
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	assertGroupStatus(t, result, "greeting", check.GroupError)
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if !strings.Contains(result.Groups[0].Err.Error(), "exit 3") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	assertGroupStatus(t, result, "later", check.GroupOK)
	if _, statErr := os.Stat(filepath.Join(root, "generated", "hello.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("hello.txt should have been wiped: %v", statErr)
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %+v", result.AllDrifts())
	}
	report, err := check.FormatFailureReport(result, root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(report, "\nDrift\n") {
		t.Fatalf("report = %s", report)
	}
	if !strings.Contains(report, "later: OK") || !strings.Contains(report, "after cleaning outputs") || !strings.Contains(report, "exit 3") {
		t.Fatalf("report = %s", report)
	}
}

func TestRunCleanRefusalSkipsCommand(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "", "", "", []testutil.GroupSpec{{
		Name:    "greeting",
		Command: `python3 -c "open('ran','w').close()"`,
		Outputs: []string{".."},
		Clean:   true,
	}}); err != nil {
		t.Fatal(err)
	}

	result := mustRunConfig(t, root)
	assertGroupStatus(t, result, "greeting", check.GroupError)
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), `clean refuses ".."`) {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if markerExists(root, "ran") {
		t.Fatal("refused clean ran the command")
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
}

func TestRunCleanRefusalLeavesLaterGroup(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "sqlc", Command: `python3 -c "open('ran','w').close()"`, Outputs: []string{"generated/hello.txt", ".."}, Clean: true},
		{Name: "wrappers", Command: "test -f generated/hello.txt", Outputs: []string{"other/store.go"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other", "store.go"), []byte("package store\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "other/store.go"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "store"); err != nil {
		t.Fatal(err)
	}

	result := mustRunConfig(t, root)
	assertGroupStatus(t, result, "sqlc", check.GroupError)
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), `clean refuses ".."`) {
		t.Fatalf("sqlc err = %v", result.Groups[0].Err)
	}
	if markerExists(root, "ran") {
		t.Fatal("refused clean ran the command")
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil {
		t.Fatalf("hello.txt removed: %v", err)
	}
	if string(got) != "hello world\n" {
		t.Fatalf("hello.txt = %q", got)
	}
	assertGroupStatus(t, result, "wrappers", check.GroupOK)
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
}

func TestRunCommandFailureLeavesRewriteWithoutDrift(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "printf 'changed\\n' > generated/hello.txt; exit 1",
		Outputs: []string{"generated/hello.txt"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustRunConfig(t, root)
	assertGroupStatus(t, result, "greeting", check.GroupError)
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "command failed (exit 1)") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if result.ExitCode() != 2 || len(result.AllDrifts()) != 0 {
		t.Fatalf("exit = %d drifts = %+v", result.ExitCode(), result.AllDrifts())
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed\n" {
		t.Fatalf("output = %q", got)
	}
	report, err := check.FormatFailureReport(result, root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(report, "\nDrift\n") || strings.Contains(report, "; drift") {
		t.Fatalf("report = %s", report)
	}
}

func TestRunTwoCommandFailures(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "first", Command: "exit 3", Outputs: []string{"generated/hello.txt"}},
		{Name: "second", Command: `python3 -c "open('second-ran','w').close(); raise SystemExit(4)"`, Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustRunConfig(t, root)
	assertGroupStatus(t, result, "first", check.GroupError)
	assertGroupStatus(t, result, "second", check.GroupError)
	if !markerExists(root, "second-ran") {
		t.Fatal("later failing group did not run")
	}
	if result.ExitCode() != 2 || len(result.AllDrifts()) != 0 {
		t.Fatalf("exit = %d drifts = %+v", result.ExitCode(), result.AllDrifts())
	}
	if !strings.Contains(result.FinalErrorLine(), "2 groups failed") {
		t.Fatalf("final = %q", result.FinalErrorLine())
	}
}

func TestRunRunsFromConfigRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "repo")
	service := filepath.Join(root, "service")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteGenerator(service); err != nil {
		t.Fatal(err)
	}
	configPath, err := testutil.WriteGenguardConfig(service, "generated/hello.txt", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "scripts/gen.py")
	cmd.Dir = service
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed generator: %v: %s", err, out)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(service, "name.txt"), []byte("genguard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "service/name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename source"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
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
	got, err := os.ReadFile(filepath.Join(service, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello genguard\n" {
		t.Fatalf("generated = %q", string(got))
	}
}

func TestRunTopLevelClean(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	content := "clean: true\n" +
		"groups:\n" +
		"  - name: greeting\n" +
		"    command: \"true\"\n" +
		"    outputs:\n" +
		"      - generated/hello.txt\n"
	if err := os.WriteFile(filepath.Join(root, "genguard.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustRunConfig(t, root)
	assertGroupStatus(t, result, "greeting", check.GroupOK)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
	if _, err := os.Stat(filepath.Join(root, "generated", "hello.txt")); !os.IsNotExist(err) {
		t.Fatalf("hello.txt = %v", err)
	}
}

const runProtoCleanProbe = `python3 -c "import pathlib,sys; p=pathlib.Path('gen/a.pb.go'); sys.exit(1) if p.exists() else (p.write_bytes(b'NEW\n'), open('proto-ran','w').close())"`

func mustRunConfig(t *testing.T, root string) check.ConfigResult {
	t.Helper()
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.RunConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func mustRunSince(t *testing.T, root, since string) check.ConfigResult {
	t.Helper()
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.RunSince(cfg, since)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func commitConfigAsBase(t *testing.T, root string, groups []testutil.GroupSpec) {
	t.Helper()
	if _, err := testutil.WriteGenguardConfig(root, "", "", "", groups); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "config"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "branch", "-f", "base"); err != nil {
		t.Fatal(err)
	}
}

func assertRunEveryGroup(t *testing.T, root string, result check.ConfigResult) {
	t.Helper()
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupOK)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if !markerExists(root, "sqlc-ran") || !markerExists(root, "proto-ran") || !markerExists(root, "plain-ran") {
		t.Fatal("run without --since skipped a group")
	}
	if result.ExitCode() != 0 || len(result.AllDrifts()) != 0 {
		t.Fatalf("exit = %d drifts = %+v", result.ExitCode(), result.AllDrifts())
	}
}
