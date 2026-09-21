package check_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/cli"
	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/internal/testutil"
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
	_, _, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	return code
}

func commitPath(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", rel); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", rel); err != nil {
		t.Fatal(err)
	}
}

func commitNameChange(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("genguard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename"); err != nil {
		t.Fatal(err)
	}
}

func mustCheckConfig(t *testing.T, root string) check.ConfigResult {
	t.Helper()
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCheckFailsWhenSourceChanged(t *testing.T) {
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

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stderr, "[modified] greeting: generated/hello.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "-hello world") || !strings.Contains(stderr, "+hello genguard") {
		t.Fatalf("stderr = %q", stderr)
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello genguard\n" {
		t.Fatalf("generated = %q", string(got))
	}
}

func TestCheckFailsWhenGeneratedDriftIsStaged(t *testing.T) {
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

	cmd := exec.Command("python3", "scripts/gen.py")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate: %v: %s", err, out)
	}
	if err := testutil.Git(root, "add", "generated/hello.txt"); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 1 {
		t.Fatalf("code = %d, want 1; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "[modified] greeting: generated/hello.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "-hello world") || !strings.Contains(stderr, "+hello genguard") {
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

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
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

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
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
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("genguard\n"), 0o644); err != nil {
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
	var genguardErr *check.GenguardError
	if !errors.As(err, &genguardErr) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "not a git work tree") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCommandEmptyCommand(t *testing.T) {
	root := t.TempDir()
	err := check.RunCommand(root, "   ")
	var genguardErr *check.GenguardError
	if !errors.As(err, &genguardErr) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "command is empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCommandFailure(t *testing.T) {
	root := t.TempDir()
	err := check.RunCommand(root, "exit 4")
	var genguardErr *check.GenguardError
	if !errors.As(err, &genguardErr) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "command failed (exit 4)") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckMultipleGroupsOnlyOneDrifts(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "greeting", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
		{Name: "noop", Command: "true", Outputs: []string{"other/out.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	commitPath(t, root, "other/out.txt", "ok\n")
	commitNameChange(t, root)

	result := mustCheckConfig(t, root)
	drifts := result.AllDrifts()
	if len(drifts) != 1 || drifts[0].Group != "greeting" || drifts[0].Kind != "modified" {
		t.Fatalf("drifts = %v", drifts)
	}
	if result.Groups[1].Status != check.GroupOK {
		t.Fatalf("noop status = %q", result.Groups[1].Status)
	}
}

func TestCheckOverlappingOutputsBothReportDrift(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "greeting", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
		{Name: "noop", Command: "true", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	commitNameChange(t, root)

	result := mustCheckConfig(t, root)
	drifts := result.AllDrifts()
	if len(drifts) != 2 {
		t.Fatalf("drifts = %v", drifts)
	}
	if drifts[0].Group != "greeting" || drifts[0].Path != "generated/hello.txt" {
		t.Fatalf("first drift = %v", drifts[0])
	}
	if drifts[1].Group != "noop" || drifts[1].Path != "generated/hello.txt" {
		t.Fatalf("second drift = %v", drifts[1])
	}
}

func TestCheckTwoCommandFailures(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "first", Command: "exit 3", Outputs: []string{"generated/hello.txt"}},
		{Name: "second", Command: "exit 4", Outputs: []string{"other/out.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if len(result.Groups) != 2 {
		t.Fatalf("groups = %d", len(result.Groups))
	}
	if result.Groups[0].Status != check.GroupError || result.Groups[1].Status != check.GroupError {
		t.Fatalf("statuses = %q, %q", result.Groups[0].Status, result.Groups[1].Status)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "exit 3") {
		t.Fatalf("first err = %v", result.Groups[0].Err)
	}
	if result.Groups[1].Err == nil || !strings.Contains(result.Groups[1].Err.Error(), "exit 4") {
		t.Fatalf("second err = %v", result.Groups[1].Err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode())
	}
}

func TestCheckDriftThenError(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "greeting", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
		{Name: "broken", Command: "exit 3", Outputs: []string{"other/out.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	commitNameChange(t, root)

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupDrift {
		t.Fatalf("greeting status = %q", result.Groups[0].Status)
	}
	if result.Groups[1].Status != check.GroupError {
		t.Fatalf("broken status = %q", result.Groups[1].Status)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode())
	}
}

func TestCheckReportsGitDiffFailureAsGroupError(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "true", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "hello.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if len(result.Groups) != 1 || result.Groups[0].Status != check.GroupError {
		t.Fatalf("groups = %+v", result.Groups)
	}
	if result.Groups[0].Err == nil {
		t.Fatal("expected git error")
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode())
	}
}

func TestCheckFlattensCommandOutput(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "broken",
		Command: `python3 -c "import sys; sys.stderr.write('line1'+chr(10)+'line2'+chr(10)); sys.exit(3)"`,
		Outputs: []string{"generated/hello.txt"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	line := result.Groups[0].SummaryLine()
	if strings.Contains(line, "\n") {
		t.Fatalf("summary contains newline: %q", line)
	}
	if !strings.Contains(line, "line1 line2") {
		t.Fatalf("summary = %q", line)
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

	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %v", result.AllDrifts())
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
	configPath, err := testutil.WriteGenguardConfig(root, "generated/missing.txt", "", "", []testutil.GroupSpec{
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

func TestDriftForGroupHandlesNewlineInFilename(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	rel := "generated/hello\nworld.txt"
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, rel, "true", "", []testutil.GroupSpec{
		{Name: "greeting", Command: "true", Outputs: []string{"generated/"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drifts, err := check.DriftForGroup(root, config.Group{
		Name:    "greeting",
		Command: "true",
		Outputs: []string{"generated/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := check.Drift{Group: "greeting", Path: rel, Kind: "modified"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %v, want [%+v]", drifts, want)
	}
}

func TestDriftForGroupSubdirectoryUsesConfigRelativePaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	api := filepath.Join(root, "api")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, "out.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	group := config.Group{Name: "api", Outputs: []string{"out.txt", "extra.txt"}}
	drifts, err := check.DriftForGroup(api, group)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 2 {
		t.Fatalf("drifts = %+v", drifts)
	}
	if drifts[0] != (check.Drift{Group: "api", Path: "out.txt", Kind: "modified"}) {
		t.Fatalf("modified = %+v", drifts[0])
	}
	if drifts[1] != (check.Drift{Group: "api", Path: "extra.txt", Kind: "untracked"}) {
		t.Fatalf("untracked = %+v", drifts[1])
	}
	diff, err := check.DriftDiff(api, drifts[:1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "diff --git") || !strings.Contains(diff, "+new") {
		t.Fatalf("diff = %q", diff)
	}

	if err := os.Remove(filepath.Join(api, "out.txt")); err != nil {
		t.Fatal(err)
	}
	drifts, err = check.DriftForGroup(api, config.Group{Name: "api", Outputs: []string{"out.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0] != (check.Drift{Group: "api", Path: "out.txt", Kind: "missing"}) {
		t.Fatalf("deleted = %+v", drifts)
	}
}

func TestDriftForGroupParentPathspec(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	api := filepath.Join(root, "api")
	web := filepath.Join(root, "web")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "config", "diff.relative", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "out.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drifts, err := check.DriftForGroup(api, config.Group{Name: "api", Outputs: []string{"../web/out.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0] != (check.Drift{Group: "api", Path: "../web/out.txt", Kind: "modified"}) {
		t.Fatalf("modified = %+v", drifts)
	}
	diff, err := check.DriftDiff(api, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "diff --git") || !strings.Contains(diff, "+new") {
		t.Fatalf("diff = %q", diff)
	}

	if err := os.WriteFile(filepath.Join(web, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drifts, err = check.DriftForGroup(api, config.Group{Name: "api", Outputs: []string{"../web/extra.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0] != (check.Drift{Group: "api", Path: "../web/extra.txt", Kind: "untracked"}) {
		t.Fatalf("untracked = %+v", drifts)
	}
	diff, err = check.DriftDiff(api, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "diff --git") || !strings.Contains(diff, "+extra") {
		t.Fatalf("diff = %q", diff)
	}

	if err := os.Remove(filepath.Join(web, "out.txt")); err != nil {
		t.Fatal(err)
	}
	drifts, err = check.DriftForGroup(api, config.Group{Name: "api", Outputs: []string{"../web/out.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0] != (check.Drift{Group: "api", Path: "../web/out.txt", Kind: "missing"}) {
		t.Fatalf("deleted = %+v", drifts)
	}
	diff, err = check.DriftDiff(api, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "deleted file") {
		t.Fatalf("diff = %q", diff)
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

	if code := runCLICheckConfig(configPath); code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	got, err := os.ReadFile(filepath.Join(service, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello genguard\n" {
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

	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %v, want none", result.AllDrifts())
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

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
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
	if _, err := testutil.WriteGenguardConfig(root, "", "exit 3", "", []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "exit 3",
		Outputs: []string{"generated/"},
		Clean:   true,
	}}); err != nil {
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
	if len(result.Groups) != 1 {
		t.Fatal("expected one group")
	}
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("status = %q, want error", result.Groups[0].Status)
	}
	if result.Groups[0].Err == nil {
		t.Fatal("expected group error")
	}
	if !strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("error = %q", result.Groups[0].Err.Error())
	}
	if !strings.Contains(result.Groups[0].Err.Error(), "exit 3") {
		t.Fatalf("error = %q", result.Groups[0].Err.Error())
	}
	if _, statErr := os.Stat(filepath.Join(root, "generated", "hello.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("hello.txt should have been wiped: %v", statErr)
	}
}

func TestCheckRunsLaterGroupAfterCleanCommandFailure(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}, Clean: true},
		{Name: "other", Command: "true", Outputs: []string{"other/out.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other", "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "other/out.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "other output"); err != nil {
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
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("broken err = %v", result.Groups[0].Err)
	}
	if result.Groups[1].Status != check.GroupOK {
		t.Fatalf("other status = %q, err = %v", result.Groups[1].Status, result.Groups[1].Err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "generated", "hello.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("hello.txt should have been wiped: %v", statErr)
	}
	got, err := os.ReadFile(filepath.Join(root, "other", "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok\n" {
		t.Fatalf("other output = %q", got)
	}
}

func TestCheckCleanFailureMakesOverlappingGroupDrift(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}, Clean: true},
		{Name: "other", Command: "true", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("broken status = %q", result.Groups[0].Status)
	}
	if result.Groups[1].Status != check.GroupDrift {
		t.Fatalf("other status = %q, err = %v, drifts = %v", result.Groups[1].Status, result.Groups[1].Err, result.Groups[1].Drifts)
	}
	if len(result.Groups[1].Drifts) != 1 || result.Groups[1].Drifts[0].Kind != "missing" {
		t.Fatalf("other drifts = %v", result.Groups[1].Drifts)
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
			if _, err := testutil.WriteGenguardConfig(root, "", "", "", []testutil.GroupSpec{{
				Name:    "greeting",
				Command: "true",
				Outputs: tc.outputs,
				Clean:   true,
			}}); err != nil {
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
			if len(result.Groups) != 1 {
				t.Fatal("expected one group")
			}
			if result.Groups[0].Status != check.GroupError {
				t.Fatalf("status = %q, want error", result.Groups[0].Status)
			}
			if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), tc.match) {
				t.Fatalf("error = %q, want substring %q", result.Groups[0].Err, tc.match)
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

	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := check.CheckConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 {
		t.Fatal("expected one group")
	}
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("status = %q, want error", result.Groups[0].Status)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "mixed tree (would delete generated/.git)") {
		t.Fatalf("error = %q", result.Groups[0].Err)
	}
}

func TestCheckCleanRefusesGitBelowOutputRoot(t *testing.T) {
	cases := []struct {
		name string
		nest func(t *testing.T, root string) string
	}{
		{
			name: "directory",
			nest: func(t *testing.T, root string) string {
				gitDir := filepath.Join(root, "generated", "sub", ".git")
				if err := os.MkdirAll(gitDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return gitDir
			},
		},
		{
			name: "gitlink",
			nest: func(t *testing.T, root string) string {
				gitFile := filepath.Join(root, "generated", "sub", ".git")
				if err := os.MkdirAll(filepath.Dir(gitFile), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(gitFile, []byte("gitdir: /tmp/elsewhere\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return gitFile
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
			gitPath := tc.nest(t, root)
			hello := filepath.Join(root, "generated", "hello.txt")

			cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := check.CheckConfig(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Groups) != 1 {
				t.Fatal("expected one group")
			}
			if result.Groups[0].Status != check.GroupError {
				t.Fatalf("status = %q, want error", result.Groups[0].Status)
			}
			if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "mixed tree (would delete generated/sub/.git)") {
				t.Fatalf("error = %q", result.Groups[0].Err)
			}
			if _, err := os.Lstat(gitPath); err != nil {
				t.Fatalf("nested git removed: %v", err)
			}
			if _, err := os.Lstat(hello); err != nil {
				t.Fatalf("generated file removed: %v", err)
			}
		})
	}
}
