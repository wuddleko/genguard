package check_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/regen-check/regen/internal/check"
	"github.com/regen-check/regen/internal/cli"
	"github.com/regen-check/regen/internal/config"
	"github.com/regen-check/regen/internal/testutil"
)

func runCLI(args []string) (stdout, stderr string, code int) {
	var outBuf, errBuf bytes.Buffer
	code = cli.RunWithIO(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

func TestCheckPassesWhenOutputMatches(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if code := runCLICheck(root); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
}

func runCLICheck(root string) int {
	_, _, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
	return code
}

func TestCheckFailsWhenSourceChanged(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("regen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename"); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "[modified] greeting: generated/hello.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "-hello world") || !strings.Contains(stderr, "+hello regen") {
		t.Fatalf("stderr = %q", stderr)
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello regen\n" {
		t.Fatalf("generated = %q", string(got))
	}
}

func TestCheckFailsWhenGeneratedDriftIsStaged(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("regen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename"); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("python3", "scripts/gen.py")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate: %v: %s", err, out)
	}
	if err := testutil.Git(root, "add", "generated/hello.txt"); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
	if code != 1 {
		t.Fatalf("code = %d, want 1; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "[modified] greeting: generated/hello.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "-hello world") || !strings.Contains(stderr, "+hello regen") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCheckFailsOnUntrackedOutput(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	genPath := filepath.Join(root, "scripts", "gen.py")
	gen, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatal(err)
	}
	extra := "(out / 'extra.txt').write_text('bonus\\n', encoding='utf-8')\n"
	if err := os.WriteFile(genPath, append(gen, []byte(extra)...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "scripts/gen.py"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "extra output"); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "[untracked] greeting: generated/extra.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "+bonus") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCheckFailsOnMissingOutput(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, "scripts", "gen.py"),
		[]byte("from pathlib import Path\nPath('generated').mkdir(exist_ok=True)\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "scripts/gen.py"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "broken generator"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "generated", "hello.txt")); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "-u", "generated/hello.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "remove output"); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "[missing] greeting: generated/hello.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestDriftDiffDeduplicatesPaths(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("regen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename"); err != nil {
		t.Fatal(err)
	}
	_ = runCLICheck(root)

	drifts := []check.Drift{
		{Group: "greeting", Path: "generated/hello.txt", Kind: "modified"},
		{Group: "greeting", Path: "generated/hello.txt", Kind: "modified"},
	}
	diff, err := check.DriftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(diff, "diff --git") != 1 {
		t.Fatalf("diff = %q", diff)
	}
}

func TestDriftDiffMissingShowsDeletion(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "generated", "hello.txt")); err != nil {
		t.Fatal(err)
	}

	diff, err := check.DriftDiff(root, []check.Drift{
		{Group: "greeting", Path: "generated/hello.txt", Kind: "missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "deleted file mode") && !strings.Contains(diff, "--- a/generated/hello.txt") {
		t.Fatalf("diff = %q", diff)
	}
}

func TestDriftDiffUntrackedDirectoryMessage(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "generated", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "child.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diff, err := check.DriftDiff(root, []check.Drift{
		{Group: "greeting", Path: "generated/nested", Kind: "untracked"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff != "Untracked generated file: generated/nested" {
		t.Fatalf("diff = %q", diff)
	}
}

func TestRequireGitRepoRaisesOutsideGit(t *testing.T) {
	root := t.TempDir()
	err := check.RequireGitRepo(root)
	var regenErr *check.RegenError
	if !errors.As(err, &regenErr) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "not a git work tree") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCommandEmptyCommand(t *testing.T) {
	root := t.TempDir()
	err := check.RunCommand(root, "   ")
	var regenErr *check.RegenError
	if !errors.As(err, &regenErr) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "command is empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCommandFailure(t *testing.T) {
	root := t.TempDir()
	err := check.RunCommand(root, "exit 4")
	var regenErr *check.RegenError
	if !errors.As(err, &regenErr) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "command failed (exit 4)") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckMultipleGroupsOnlyOneDrifts(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "greeting", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
		{Name: "noop", Command: "true", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("regen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "regen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	drifts, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var greeting []check.Drift
	for _, drift := range drifts {
		if drift.Group == "greeting" {
			greeting = append(greeting, drift)
		}
	}
	if len(greeting) != 1 || greeting[0].Kind != "modified" {
		t.Fatalf("greeting drifts = %v", greeting)
	}
}

func TestCheckMultipleGroupsAllPass(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "one", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
		{Name: "two", Command: "true", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "regen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	drifts, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 0 {
		t.Fatalf("drifts = %v", drifts)
	}
}

func TestDriftForGroupSkipsMissingCheckForGlob(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/*.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	group := config.Group{
		Name:    "greeting",
		Command: "python3 scripts/gen.py",
		Outputs: []string{"generated/*.txt"},
	}
	drifts, err := check.DriftForGroup(root, group)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 0 {
		t.Fatalf("drifts = %v", drifts)
	}
}

func TestDriftForGroupSkipsMissingCheckForDirectory(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	group := config.Group{
		Name:    "greeting",
		Command: "python3 scripts/gen.py",
		Outputs: []string{"generated/"},
	}
	drifts, err := check.DriftForGroup(root, group)
	if err != nil {
		t.Fatal(err)
	}
	for _, drift := range drifts {
		if drift.Kind == "missing" {
			t.Fatalf("unexpected missing drift: %+v", drift)
		}
	}
}

func TestDriftForGroupReportsMissingFileSpec(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	configPath, err := testutil.WriteRegenConfig(root, "generated/missing.txt", "", "", []testutil.GroupSpec{
		{Name: "greeting", Command: "true", Outputs: []string{"generated/missing.txt"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	drifts, err := check.DriftForGroup(root, cfg.Groups[0])
	if err != nil {
		t.Fatal(err)
	}
	want := check.Drift{Group: "greeting", Path: "generated/missing.txt", Kind: "missing"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %v, want [%+v]", drifts, want)
	}
}

func TestDriftForGroupDeletedTrackedFileIsMissingOnce(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "generated", "hello.txt")); err != nil {
		t.Fatal(err)
	}

	drifts, err := check.DriftForGroup(root, config.Group{
		Name:    "greeting",
		Command: "true",
		Outputs: []string{"generated/hello.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := check.Drift{Group: "greeting", Path: "generated/hello.txt", Kind: "missing"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %v, want [%+v]", drifts, want)
	}
}

func TestCheckRunsFromConfigRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "repo")
	service := filepath.Join(root, "service")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := testutil.WriteGenerator(service); err != nil {
		t.Fatal(err)
	}
	configPath, err := testutil.WriteRegenConfig(service, "generated/hello.txt", "", "", nil)
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
	if err := os.WriteFile(filepath.Join(service, "name.txt"), []byte("regen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "service/name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename source"); err != nil {
		t.Fatal(err)
	}

	if code := runCLICheckConfig(configPath); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	got, err := os.ReadFile(filepath.Join(service, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello regen\n" {
		t.Fatalf("generated = %q", string(got))
	}
}

func runCLICheckConfig(configPath string) int {
	_, _, code := runCLI([]string{"check", "--config", configPath})
	return code
}

func TestGreetingDirectoryOutputLayout(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if code := runCLICheck(root); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
}

func commitOrphanGeneratedFile(t *testing.T, root string) {
	t.Helper()
	extra := filepath.Join(root, "generated", "extra.txt")
	if err := os.WriteFile(extra, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "generated/extra.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "orphan"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckWithoutCleanIgnoresOrphan(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	commitOrphanGeneratedFile(t, root)

	cfg, err := config.LoadConfig(filepath.Join(root, "regen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	drifts, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 0 {
		t.Fatalf("drifts = %v, want none", drifts)
	}
}

func TestCheckCleanFailsOnOrphan(t *testing.T) {
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

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
	if code != 1 {
		t.Fatalf("code = %d, want 1; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "generated/extra.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "generated", "extra.txt")); !os.IsNotExist(err) {
		t.Fatalf("orphan still on disk: %v", err)
	}
}

func TestCheckCleanPassesWhenOutputsMatch(t *testing.T) {
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
	if code := runCLICheck(root); code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
}

func TestCheckCleanCommandFailureAfterWipe(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteRegenConfig(root, "", "exit 3", "", []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "exit 3",
		Outputs: []string{"generated/"},
		Clean:   true,
	}}); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "regen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = check.CheckConfig(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "after cleaning outputs") {
		t.Fatalf("error = %q", err)
	}
	if !strings.Contains(err.Error(), "exit 3") {
		t.Fatalf("error = %q", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "generated", "hello.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("hello.txt should have been wiped: %v", statErr)
	}
}

func TestCheckCleanRefusesUnsafeOutputs(t *testing.T) {
	cases := []struct {
		outputs []string
		match   string
	}{
		{[]string{"."}, `clean refuses "."`},
		{[]string{".."}, `clean refuses ".."`},
		{[]string{"generated/*.txt"}, "glob"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(strings.Join(tc.outputs, ","), func(t *testing.T) {
			root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := testutil.WriteRegenConfig(root, "", "", "", []testutil.GroupSpec{{
				Name:    "greeting",
				Command: "true",
				Outputs: tc.outputs,
				Clean:   true,
			}}); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.LoadConfig(filepath.Join(root, "regen.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			_, err = check.CheckConfig(cfg)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.match) {
				t.Fatalf("error = %q, want substring %q", err, tc.match)
			}
		})
	}
}

func TestCheckCleanRefusesNestedGit(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "true",
		Outputs: []string{"generated/"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	nestedGit := filepath.Join(root, "generated", ".git")
	if err := os.Mkdir(nestedGit, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "regen.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = check.CheckConfig(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "mixed tree") {
		t.Fatalf("error = %q", err)
	}
}
