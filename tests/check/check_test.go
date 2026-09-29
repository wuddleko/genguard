package check_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/cli"
	"github.com/wuddleko/genguard/internal/command"
	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestMain(m *testing.M) {
	// CI sets these on the test process. Annotation tests opt in with t.Setenv.
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("GENGUARD_ANNOTATIONS")
	os.Unsetenv("GITHUB_WORKSPACE")
	os.Exit(m.Run())
}

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

func mustCheckConfig(t *testing.T, root string) check.ConfigResult {
	t.Helper()
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := checkConfig(cfg)
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
	extra := "(out / 'extra.txt').write_bytes(b'bonus\\n')\n"
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

func TestRunCommandEmptyCommand(t *testing.T) {
	root := t.TempDir()
	tail, err := command.Run(context.Background(), root, "   ", nil, 0)
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if err == nil || !strings.Contains(err.Error(), "command is empty") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCommandFailure(t *testing.T) {
	root := t.TempDir()
	tail, err := command.Run(context.Background(), root, "exit 4", nil, 0)
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if err == nil || err.Error() != "command failed (exit 4): no output" {
		t.Fatalf("err = %v", err)
	}
}

func TestRunCommandFailureReturnsTail(t *testing.T) {
	root := t.TempDir()
	tail, err := command.Run(context.Background(), root, `python3 -c "import sys; sys.stderr.write('line1'+chr(10)+'line2'+chr(10)); sys.exit(3)"`, nil, 0)
	if err == nil || err.Error() != "command failed (exit 3)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "line1\nline2\n" {
		t.Fatalf("tail = %q", tail)
	}
}

func TestRunCommandSuccessDropsOutput(t *testing.T) {
	root := t.TempDir()
	tail, err := command.Run(context.Background(), root, `python3 -c "import sys; sys.stderr.write('hello'+chr(10))"`, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
}

func TestRunCommandStartFailure(t *testing.T) {
	root := t.TempDir()
	tail, err := command.Run(context.Background(), filepath.Join(root, "missing"), "true", nil, 0)
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if err == nil || !strings.HasPrefix(err.Error(), "command failed to start:") {
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
	testutil.CommitPath(t, root, "other/out.txt", "ok\n")
	testutil.CommitNameChange(t, root)

	result := mustCheckConfig(t, root)
	drifts := result.AllDrifts()
	if len(drifts) != 1 || drifts[0].Group != "greeting" || drifts[0].Kind != "modified" {
		t.Fatalf("drifts = %v", drifts)
	}
	if result.Groups[1].Status != check.GroupOK {
		t.Fatalf("noop status = %q", result.Groups[1].Status)
	}
}

func TestCheckOverlappingOutputsFailAtLoad(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "greeting", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
		{Name: "noop", Command: "true", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	_, err = config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), `outputs overlap: group "greeting" "generated/hello.txt" and group "noop" "generated/hello.txt"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckDirectoryPrefixOverlapExitsBeforeDrift(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "dir", Command: "printf 'new\\n' > gen/out.go", Outputs: []string{"gen/"}},
		{Name: "file", Command: "true", Outputs: []string{"gen/out.go"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "gen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gen", "out.go"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "gen/out.go"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "gen"); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stdout = %q stderr = %q", code, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stderr, "Drift") || strings.Contains(stderr, "[modified]") || strings.Contains(stderr, "[missing]") {
		t.Fatalf("stderr reported drift:\n%s", stderr)
	}
	for _, want := range []string{`group "dir" "gen/"`, `group "file" "gen/out.go"`} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, missing %q", stderr, want)
		}
	}
}

func TestCheckRootGlobAndDirectoryOverlapExitsBeforeCommand(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	content := "groups:\n" +
		"  - name: protobuf\n" +
		"    command: touch protobuf-ran\n" +
		"    outputs:\n" +
		"      - pkg/gen/\n" +
		"  - name: templ\n" +
		"    command: touch templ-ran\n" +
		"    outputs:\n" +
		"      - \"*_templ.go\"\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI([]string{"check", "--config", configPath})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stdout = %q stderr = %q", code, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	for _, want := range []string{`group "protobuf" "pkg/gen/"`, `group "templ" "*_templ.go"`} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr = %q, missing %q", stderr, want)
		}
	}
	for _, name := range []string{"protobuf-ran", "templ-ran"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s exists or stat failed: %v", name, err)
		}
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
	testutil.CommitNameChange(t, root)

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
	if result.Groups[0].Err == nil || result.Groups[0].Err.Error() != "command failed (exit 3)" {
		t.Fatalf("err = %v", result.Groups[0].Err)
	}
	line := result.Groups[0].SummaryLine()
	if strings.Contains(line, "\n") || strings.Contains(line, "line1") || !strings.Contains(line, "command failed (exit 3)") {
		t.Fatalf("summary = %q", line)
	}
	if result.Groups[0].CommandTail != "line1\nline2\n" {
		t.Fatalf("tail = %q", result.Groups[0].CommandTail)
	}
}

func TestCommandTailStaysEmpty(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		groups := []testutil.GroupSpec{{
			Name:    "ok",
			Command: `python3 -c "import sys; sys.stderr.write('printed'+chr(10))"`,
			Outputs: []string{"generated/hello.txt"},
		}}
		root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
		if err != nil {
			t.Fatal(err)
		}
		result := mustCheckConfig(t, root)
		if result.Groups[0].Status != check.GroupOK {
			t.Fatalf("status = %q", result.Groups[0].Status)
		}
		if result.Groups[0].CommandTail != "" {
			t.Fatalf("tail = %q", result.Groups[0].CommandTail)
		}
	})

	t.Run("skip", func(t *testing.T) {
		groups := []testutil.GroupSpec{{
			Name:    "quiet",
			Command: `python3 -c "import sys; sys.stderr.write('ran'+chr(10)); raise SystemExit(3)"`,
			Inputs:  []string{"queries/"},
			Outputs: []string{"out.txt"},
		}}
		root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
		if err != nil {
			t.Fatal(err)
		}
		testutil.CommitPath(t, root, "queries/q.sql", "select 1;\n")
		testutil.CommitPath(t, root, "out.txt", "out\n")
		if err := testutil.Git(root, "branch", "base"); err != nil {
			t.Fatal(err)
		}
		result := mustCheckSince(t, root, "base")
		if result.Groups[0].Status != check.GroupSkipped {
			t.Fatalf("status = %q", result.Groups[0].Status)
		}
		if result.Groups[0].CommandTail != "" {
			t.Fatalf("tail = %q", result.Groups[0].CommandTail)
		}
	})

	t.Run("drift", func(t *testing.T) {
		groups := []testutil.GroupSpec{{
			Name:    "greeting",
			Command: `python3 -c "import sys; sys.stderr.write('printed'+chr(10)); open('generated/hello.txt','w').write('changed\n')"`,
			Outputs: []string{"generated/hello.txt"},
		}}
		root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
		if err != nil {
			t.Fatal(err)
		}
		result := mustCheckConfig(t, root)
		if result.Groups[0].Status != check.GroupDrift {
			t.Fatalf("status = %q, err = %v", result.Groups[0].Status, result.Groups[0].Err)
		}
		if result.Groups[0].CommandTail != "" {
			t.Fatalf("tail = %q", result.Groups[0].CommandTail)
		}
	})
}

func TestRunStoresCommandTail(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "broken",
		Command: `python3 -c "import sys; sys.stderr.write('line1'+chr(10)+'line2'+chr(10)); sys.exit(3)"`,
		Outputs: []string{"generated/hello.txt"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("err = %v", result.Groups[0].Err)
	}
	if result.Groups[0].CommandTail != "line1\nline2\n" {
		t.Fatalf("tail = %q", result.Groups[0].CommandTail)
	}
}

func TestCheckMultipleGroupsAllPass(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "one", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
		{Name: "two", Command: "true", Outputs: []string{"other/out.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", groups)
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
	if err := testutil.Git(root, "commit", "-m", "second output"); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := checkConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AllDrifts()) != 0 {
		t.Fatalf("drifts = %v", result.AllDrifts())
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
	result, err := checkConfig(cfg)
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
	result, err := checkConfig(cfg)
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
	if len(result.Groups[0].Drifts) != 0 {
		t.Fatalf("wipe residue drifts = %v", result.Groups[0].Drifts)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
}

func TestCheckCommandFailureStillReportsDrift(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "printf 'changed\\n' > generated/hello.txt; exit 1",
		Outputs: []string{"generated/hello.txt"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("status = %q, want error", result.Groups[0].Status)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "command failed (exit 1)") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	if len(result.Groups[0].Drifts) != 1 || result.Groups[0].Drifts[0].Kind != "modified" || result.Groups[0].Drifts[0].Path != "generated/hello.txt" {
		t.Fatalf("drifts = %v", result.Groups[0].Drifts)
	}
	report, err := singleReport(result, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "[modified] greeting: generated/hello.txt") {
		t.Fatalf("report = %s", report)
	}
	if !strings.Contains(report, "  greeting: error (command failed (exit 1): no output); drift (1 modified)") {
		t.Fatalf("report = %s", report)
	}
	if !strings.Contains(report, "1 group: 0 ok, 0 drift, 1 error") {
		t.Fatalf("report = %s", report)
	}
	if !strings.Contains(report, "\nDrift\n") {
		t.Fatalf("report = %s", report)
	}
	if !strings.Contains(result.FinalErrorLine(), "command failed (exit 1)") || strings.Contains(result.FinalErrorLine(), "group drifted") {
		t.Fatalf("final = %q", result.FinalErrorLine())
	}
}

func TestCheckExit3TouchesNothing(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "exit 3",
		Outputs: []string{"generated/hello.txt"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("status = %q, want error", result.Groups[0].Status)
	}
	if len(result.Groups[0].Drifts) != 0 {
		t.Fatalf("drifts = %v", result.Groups[0].Drifts)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	report, err := singleReport(result, root)
	if err != nil {
		t.Fatal(err)
	}
	const want = "Summary\n" +
		"  greeting: error (command failed (exit 3): no output)\n" +
		"1 group: 0 ok, 0 drift, 1 error\n" +
		"\n" +
		"error: command failed (exit 3): no output\n"
	if report != want {
		t.Fatalf("report = %q, want %q", report, want)
	}
}

func TestCheckErrorWithDriftPlusOtherDrift(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "greeting", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
		{Name: "broken", Command: "printf 'changed\\n' > other/out.txt; exit 1", Outputs: []string{"other/out.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CommitPath(t, root, "other/out.txt", "ok\n")
	testutil.CommitNameChange(t, root)

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupDrift {
		t.Fatalf("greeting status = %q", result.Groups[0].Status)
	}
	if result.Groups[1].Status != check.GroupError {
		t.Fatalf("broken status = %q", result.Groups[1].Status)
	}
	if !strings.Contains(result.Groups[1].SummaryLine(), "); drift (1 modified)") {
		t.Fatalf("summary = %q", result.Groups[1].SummaryLine())
	}
	lines := result.SummaryLines()
	if lines[len(lines)-1] != "2 groups: 0 ok, 1 drift, 1 error" {
		t.Fatalf("totals = %q", lines[len(lines)-1])
	}
	if result.FinalErrorLine() != "error: 1 group failed; 1 group drifted" {
		t.Fatalf("final = %q", result.FinalErrorLine())
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
}

func TestCheckCleanCommandFailureReportsRewrite(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "printf 'changed\\n' > generated/hello.txt; exit 1",
		Outputs: []string{"generated/hello.txt"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("status = %q, want error", result.Groups[0].Status)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	if len(result.Groups[0].Drifts) != 1 || result.Groups[0].Drifts[0].Kind != "modified" || result.Groups[0].Drifts[0].Path != "generated/hello.txt" {
		t.Fatalf("drifts = %v", result.Groups[0].Drifts)
	}
	const wantSummary = "  greeting: error (command failed after cleaning outputs: command failed (exit 1): no output); drift (1 modified)"
	if result.Groups[0].SummaryLine() != wantSummary {
		t.Fatalf("summary = %q", result.Groups[0].SummaryLine())
	}
}

func TestCheckCleanCommandFailureReportsRewriteNotSiblingWipe(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "greeting",
		Command: "printf 'changed\\n' > generated/hello.txt; exit 1",
		Outputs: []string{"generated/hello.txt", "generated/other.txt"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	testutil.CommitPath(t, root, "generated/other.txt", "other\n")

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("status = %q, want error", result.Groups[0].Status)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("error = %v", result.Groups[0].Err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	if len(result.Groups[0].Drifts) != 1 || result.Groups[0].Drifts[0].Kind != "modified" || result.Groups[0].Drifts[0].Path != "generated/hello.txt" {
		t.Fatalf("drifts = %v", result.Groups[0].Drifts)
	}
	if _, statErr := os.Stat(filepath.Join(root, "generated", "other.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("other.txt should have been wiped: %v", statErr)
	}
}

func TestCheckFailedCleanRewriteVisibleAndLaterGroupOmitsWipe(t *testing.T) {
	groups := []testutil.GroupSpec{
		{
			Name:    "broken",
			Command: "printf 'changed\\n' > generated/hello.txt; exit 1",
			Outputs: []string{"generated/hello.txt", "generated/other.txt"},
			Clean:   true,
		},
		{
			Name:    "later",
			Command: "true",
			Outputs: []string{"generated/other.txt"},
		},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	_, err = config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), `group "broken" "generated/other.txt" and group "later" "generated/other.txt"`) {
		t.Fatalf("error = %v", err)
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
	result, err := checkConfig(cfg)
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

func TestCheckCleanFailureDoesNotBlameOverlappingGroup(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}, Clean: true},
		{Name: "other", Command: "true", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	_, err = config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), `group "broken" "generated/hello.txt" and group "other" "generated/hello.txt"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckCleanFailureDoesNotBlameLaterCommandFailure(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}, Clean: true},
		{Name: "other", Command: "exit 1", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	_, err = config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), `group "broken" "generated/hello.txt" and group "other" "generated/hello.txt"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckCleanFailureStillReportsLaterRewrite(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}, Clean: true},
		{Name: "other", Command: "printf 'other\\n' > generated/hello.txt", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	_, err = config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), `group "broken" "generated/hello.txt" and group "other" "generated/hello.txt"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckCleanFailureRestoreThenLaterWipe(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}, Clean: true},
		{Name: "restore", Command: "printf 'hello world\\n' > generated/hello.txt", Outputs: []string{"generated/hello.txt"}},
		{Name: "wiper", Command: "true", Outputs: []string{"generated/hello.txt"}, Clean: true},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}

	_, err = config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err == nil || !strings.Contains(err.Error(), `group "broken" "generated/hello.txt" and group "restore" "generated/hello.txt"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCheckCleanFailureKeepsUnrelatedLaterDrift(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(root, "other", "out.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[1].Status != check.GroupDrift {
		t.Fatalf("other status = %q, drifts = %v", result.Groups[1].Status, result.Groups[1].Drifts)
	}
	if len(result.Groups[1].Drifts) != 1 || result.Groups[1].Drifts[0].Path != "other/out.txt" || result.Groups[1].Drifts[0].Kind != "modified" {
		t.Fatalf("other drifts = %v", result.Groups[1].Drifts)
	}
}

func TestCheckCleanRefusesUnsafeOutputs(t *testing.T) {
	cases := []struct {
		outputs []string
		match   string
	}{
		{[]string{"."}, `clean refuses "."`},
		{[]string{".."}, `clean refuses ".."`},
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
			result, err := checkConfig(cfg)
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

func TestCheckCleanRefusalLeavesLaterGroup(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "sqlc", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt", "../outside.txt"}, Clean: true},
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

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("sqlc status = %q, err = %v", result.Groups[0].Status, result.Groups[0].Err)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), `clean refuses "../outside.txt"`) {
		t.Fatalf("sqlc err = %v", result.Groups[0].Err)
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil {
		t.Fatalf("hello.txt removed: %v", err)
	}
	if string(got) != "hello world\n" {
		t.Fatalf("hello.txt = %q", got)
	}
	if result.Groups[1].Status != check.GroupOK {
		t.Fatalf("wrappers status = %q, err = %v", result.Groups[1].Status, result.Groups[1].Err)
	}
}

func TestCheckCleanGlobLeavesUnmatchedFile(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "sqlc",
		Command: "python3 scripts/gen.py && printf 'package q\\n' > generated/oidc_queries.sql.go",
		Outputs: []string{"generated/*_queries.sql.go"},
		Clean:   true,
	}, {
		Name:    "wrappers",
		Command: "test -f generated/hello.txt && test -f generated/oidc_queries.sql.go && test -f generated/generate.go",
		Outputs: []string{"other/store.go"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "oidc_queries.sql.go"), []byte("package q\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "generate.go"), []byte("package hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other", "store.go"), []byte("package store\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "outputs"); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupOK {
		t.Fatalf("sqlc status = %q, err = %v, drifts = %v", result.Groups[0].Status, result.Groups[0].Err, result.Groups[0].Drifts)
	}
	if result.Groups[1].Status != check.GroupOK {
		t.Fatalf("wrappers status = %q, err = %v", result.Groups[1].Status, result.Groups[1].Err)
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "generate.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package hand\n" {
		t.Fatalf("generate.go = %q", got)
	}
}

func TestCheckCleanGlobReportsStaleFile(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "sqlc",
		Command: "python3 scripts/gen.py",
		Outputs: []string{"generated/*.txt"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "old.txt"), []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "keep.go"), []byte("package hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "generated/old.txt", "generated/keep.go"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "stale"); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupDrift {
		t.Fatalf("status = %q, err = %v, drifts = %v", result.Groups[0].Status, result.Groups[0].Err, result.Groups[0].Drifts)
	}
	if len(result.Groups[0].Drifts) != 1 || result.Groups[0].Drifts[0].Path != "generated/old.txt" || result.Groups[0].Drifts[0].Kind != "missing" {
		t.Fatalf("drifts = %v", result.Groups[0].Drifts)
	}
	if _, err := os.Lstat(filepath.Join(root, "generated", "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old.txt should be removed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "generated", "keep.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package hand\n" {
		t.Fatalf("keep.go = %q", got)
	}
}

func TestCheckCleanGlobSqlcPackage(t *testing.T) {
	oidc := "package db\n\nfunc Queries() {}\n"
	session := "package db\n\ntype OidcSession struct{}\n"
	groups := []testutil.GroupSpec{{
		Name:    "sqlc",
		Command: `python3 -c 'import os; os.makedirs("query", exist_ok=True); open("query/oidc_queries.sql.go","wb").write(b"package db\n\nfunc Queries() {}\n"); open("query/session_queries.sql.go","wb").write(b"package db\n\ntype OidcSession struct{}\n")'`,
		Outputs: []string{"query/*_queries.sql.go"},
		Clean:   true,
	}, {
		Name:    "wrappers",
		Command: "test -f db.go && test -f models.go && test -f query/oidc_queries.sql.go && test -f query/session_queries.sql.go",
		Outputs: []string{"wrapper.go"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	db := "package db\n\nfunc Queries() {}\n"
	models := "package db\n\ntype OidcSession struct{}\n"
	for name, body := range map[string]string{
		"db.go":      db,
		"models.go":  models,
		"wrapper.go": "package wrap\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "query"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"oidc_queries.sql.go":    oidc,
		"session_queries.sql.go": session,
	} {
		if err := os.WriteFile(filepath.Join(root, "query", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "package"); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupOK {
		t.Fatalf("sqlc status = %q, err = %v, drifts = %v", result.Groups[0].Status, result.Groups[0].Err, result.Groups[0].Drifts)
	}
	if result.Groups[1].Status != check.GroupOK {
		t.Fatalf("wrappers status = %q, err = %v", result.Groups[1].Status, result.Groups[1].Err)
	}
	got, err := os.ReadFile(filepath.Join(root, "db.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != db {
		t.Fatalf("db.go = %q", got)
	}
	got, err = os.ReadFile(filepath.Join(root, "models.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != models {
		t.Fatalf("models.go = %q", got)
	}
}

func TestCheckCleanGlobStaleQueryFile(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "sqlc",
		Command: "true",
		Outputs: []string{"*_queries.sql.go"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "db.go"), []byte("package db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "models.go"), []byte("package db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "old_queries.sql.go"), []byte("package old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "db.go", "models.go", "old_queries.sql.go"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "stale query"); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupDrift {
		t.Fatalf("status = %q, err = %v, drifts = %v", result.Groups[0].Status, result.Groups[0].Err, result.Groups[0].Drifts)
	}
	if len(result.Groups[0].Drifts) != 1 || result.Groups[0].Drifts[0].Path != "old_queries.sql.go" || result.Groups[0].Drifts[0].Kind != "missing" {
		t.Fatalf("drifts = %v", result.Groups[0].Drifts)
	}
	if _, err := os.Lstat(filepath.Join(root, "old_queries.sql.go")); !os.IsNotExist(err) {
		t.Fatalf("old query file should be removed: %v", err)
	}
	for _, name := range []string{"db.go", "models.go"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s removed: %v", name, err)
		}
	}
}

func TestCheckCleanGlobCommandFailureLeavesHandWrittenFiles(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "sqlc",
		Command: "exit 3",
		Outputs: []string{"query/*_queries.sql.go"},
		Clean:   true,
	}, {
		Name:    "wrappers",
		Command: "test -f db.go && test -f models.go",
		Outputs: []string{"wrapper.go"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"db.go":      "package db\n",
		"models.go":  "package db\n",
		"wrapper.go": "package wrap\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "query"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "query", "oidc_queries.sql.go"), []byte("package db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "db.go", "models.go", "query/oidc_queries.sql.go", "wrapper.go"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "package"); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("sqlc status = %q, err = %v", result.Groups[0].Status, result.Groups[0].Err)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "after cleaning outputs") || !strings.Contains(result.Groups[0].Err.Error(), "exit 3") {
		t.Fatalf("sqlc err = %v", result.Groups[0].Err)
	}
	if _, err := os.Lstat(filepath.Join(root, "query", "oidc_queries.sql.go")); !os.IsNotExist(err) {
		t.Fatalf("query file should be removed: %v", err)
	}
	for _, name := range []string{"db.go", "models.go"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s removed: %v", name, err)
		}
	}
	if result.Groups[1].Status != check.GroupOK {
		t.Fatalf("wrappers status = %q, err = %v", result.Groups[1].Status, result.Groups[1].Err)
	}
}

func TestCheckCleanGlobEmptyMatchRunsCommand(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "sqlc",
		Command: "mkdir -p extra && printf 'n\\n' > extra/new.txt",
		Outputs: []string{"extra/*.txt"},
		Clean:   true,
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "db.go"), []byte("package db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "db.go"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "db"); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupDrift {
		t.Fatalf("status = %q, err = %v, drifts = %v", result.Groups[0].Status, result.Groups[0].Err, result.Groups[0].Drifts)
	}
	if len(result.Groups[0].Drifts) != 1 || result.Groups[0].Drifts[0].Path != "extra/new.txt" || result.Groups[0].Drifts[0].Kind != "untracked" {
		t.Fatalf("drifts = %v", result.Groups[0].Drifts)
	}
	if _, err := os.Lstat(filepath.Join(root, "db.go")); err != nil {
		t.Fatalf("db.go removed: %v", err)
	}
}

func TestCheckCleanGlobSymlinkPrefixSkipsCommand(t *testing.T) {
	groups := []testutil.GroupSpec{{
		Name:    "sqlc",
		Command: "printf 'pwn\\n' > generated/pwned.txt",
		Outputs: []string{"generated/*.txt"},
		Clean:   true,
	}, {
		Name:    "wrappers",
		Command: "test -f db.go",
		Outputs: []string{"wrapper.go"},
	}}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "db.go"), []byte("package db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wrapper.go"), []byte("package wrap\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "generated")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "generated")); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "db.go", "wrapper.go"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "package"); err != nil {
		t.Fatal(err)
	}

	result := mustCheckConfig(t, root)
	if result.Groups[0].Status != check.GroupError {
		t.Fatalf("sqlc status = %q, err = %v", result.Groups[0].Status, result.Groups[0].Err)
	}
	if result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "symlink") {
		t.Fatalf("sqlc err = %v", result.Groups[0].Err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "pwned.txt")); !os.IsNotExist(err) {
		t.Fatalf("command wrote through symlink: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(outside, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret\n" {
		t.Fatalf("secret = %q", got)
	}
	if result.Groups[1].Status != check.GroupOK {
		t.Fatalf("wrappers status = %q, err = %v", result.Groups[1].Status, result.Groups[1].Err)
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
	result, err := checkConfig(cfg)
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
			result, err := checkConfig(cfg)
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

func TestSinceSkipsUnchangedGroupsAndDoesNotClean(t *testing.T) {
	root := writeSinceRepo(t)
	result := mustCheckSince(t, root, "base")

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
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d", result.ExitCode())
	}
}

func TestSinceRunsChangedInput(t *testing.T) {
	root := writeSinceRepo(t)
	testutil.CommitPath(t, root, "queries/q.sql", "select 2;\n")

	result := mustCheckSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}
	if !markerExists(root, "sqlc-ran") {
		t.Fatal("changed input did not run sqlc")
	}
	if markerExists(root, "proto-ran") {
		t.Fatal("unchanged protobuf ran")
	}
	if _, err := os.Stat(filepath.Join(root, "gen", "a.pb.go")); err != nil {
		t.Fatal(err)
	}
}

func TestSinceRunsHandEditedOutput(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "internal", "db", "out.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustCheckSince(t, root, "HEAD")
	sqlc := groupResult(t, result, "sqlc")
	if sqlc.Status != check.GroupDrift {
		t.Fatalf("sqlc status = %q, want drift", sqlc.Status)
	}
	if len(sqlc.Drifts) != 1 || sqlc.Drifts[0].Kind != "modified" || sqlc.Drifts[0].Path != "internal/db/out.txt" {
		t.Fatalf("drifts = %+v", sqlc.Drifts)
	}
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if result.ExitCode() != 1 {
		t.Fatalf("exit = %d, want 1", result.ExitCode())
	}
	if !strings.Contains(strings.Join(result.SummaryLines(), "\n"), "  protobuf: skipped") {
		t.Fatal("skipped group was not labeled skipped")
	}
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("output edit did not select only sqlc")
	}
}

func TestSinceUntrackedInputRuns(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "queries", "new.sql"), []byte("select 3;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := mustCheckSince(t, root, "HEAD")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") {
		t.Fatal("untracked input did not run sqlc")
	}
}

func TestSinceIgnoredInputDoesNotRun(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "queries", "skip.ignore"), []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := mustCheckSince(t, root, "HEAD")
	assertGroupStatus(t, result, "sqlc", check.GroupSkipped)
	if markerExists(root, "sqlc-ran") {
		t.Fatal("gitignored input ran sqlc")
	}
}

func TestCheckWithoutSinceRunsEveryGroup(t *testing.T) {
	root := writeSinceRepo(t)
	result := mustCheckConfig(t, root)
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupOK)
	assertGroupStatus(t, result, "plain", check.GroupOK)
	if !markerExists(root, "sqlc-ran") || !markerExists(root, "proto-ran") || !markerExists(root, "plain-ran") {
		t.Fatal("check without --since skipped a group")
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", result.ExitCode())
	}

	blank := mustCheckSince(t, root, "")
	assertGroupStatus(t, blank, "sqlc", check.GroupOK)
	assertGroupStatus(t, blank, "protobuf", check.GroupOK)
	assertGroupStatus(t, blank, "plain", check.GroupOK)
	if blank.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", blank.ExitCode())
	}
}

func TestSinceBadRef(t *testing.T) {
	root := writeSinceRepo(t)
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = checkSince(cfg, "not-a-ref")
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("err = %v", err)
	}
	if markerExists(root, "plain-ran") {
		t.Fatal("bad ref ran a group")
	}
}

func TestSinceCheckAllSelectsPerGroup(t *testing.T) {
	root := writeSinceRepo(t)
	testutil.CommitPath(t, root, "queries/q.sql", "select 2;\n")

	run, err := check.ExecuteAll(check.Options{RepoRoot: root, Since: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Configs) != 2 {
		t.Fatalf("configs = %d", len(run.Configs))
	}
	sqlc := groupInRun(t, run, "sqlc")
	protobuf := groupInRun(t, run, "protobuf")
	api := groupInRun(t, run, "api")
	if sqlc.Status != check.GroupOK || protobuf.Status != check.GroupSkipped || api.Status != check.GroupSkipped {
		t.Fatalf("sqlc=%s protobuf=%s api=%s", sqlc.Status, protobuf.Status, api.Status)
	}
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") || markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("check --all did not select per group")
	}
	if _, err := os.Stat(filepath.Join(root, "gen", "a.pb.go")); err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d", run.ExitCode())
	}
}

func TestSinceCheckAllBadRef(t *testing.T) {
	root := writeSinceRepo(t)
	_, err := check.ExecuteAll(check.Options{RepoRoot: root, Since: "not-a-ref"})
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("err = %v", err)
	}
	if markerExists(root, "plain-ran") || markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("bad ref ran a group")
	}
}

func TestCLISinceReportsSkipAndBadRef(t *testing.T) {
	root := writeSinceRepo(t)
	testutil.CommitPath(t, root, "queries/q.sql", "select 2;\n")

	stdout, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml"), "--since", "base"})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "Generated files match the generators.") || !strings.Contains(stdout, "sqlc: OK") || !strings.Contains(stdout, "protobuf: skipped") {
		t.Fatalf("stdout = %q", stdout)
	}

	testutil.Chdir(t, root)
	stdout, stderr, code = runCLI([]string{"check", "--all", "--since", "not-a-ref"})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if stdout != "" || !strings.Contains(stderr, "bad --since ref") {
		t.Fatalf("stdout = %q stderr = %q", stdout, stderr)
	}
}

func TestCLISinceDashRef(t *testing.T) {
	root := writeSinceRepo(t)
	cfg := filepath.Join(root, "genguard.yaml")
	for _, since := range []string{"--all", "-n"} {
		for _, isolated := range []bool{false, true} {
			args := []string{"check", "--config", cfg, "--since", since}
			if isolated {
				args = []string{"check", "--isolated", "--config", cfg, "--since", since}
			}
			stdout, stderr, code := runCLI(args)
			if code != 2 || stdout != "" || !strings.Contains(stderr, "bad --since ref") {
				t.Fatalf("args %q: code = %d stdout = %q stderr = %q", args, code, stdout, stderr)
			}
			if strings.Contains(stderr, "unknown option") || strings.Contains(stderr, "unrecognized argument") {
				t.Fatalf("args %q: stderr = %q", args, stderr)
			}
		}
	}
}

func TestSinceRunsWhenWorktreeMatchesBaseNotHEAD(t *testing.T) {
	root := writeSinceRepo(t)
	testutil.CommitPath(t, root, "internal/db/out.txt", "edited\n")
	if err := os.WriteFile(filepath.Join(root, "internal/db/out.txt"), []byte("db\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustCheckSince(t, root, "base")
	sqlc := groupResult(t, result, "sqlc")
	if sqlc.Status != check.GroupDrift {
		t.Fatalf("sqlc status = %q, want drift; err = %v", sqlc.Status, sqlc.Err)
	}
	if len(sqlc.Drifts) != 1 || sqlc.Drifts[0].Kind != "modified" || sqlc.Drifts[0].Path != "internal/db/out.txt" {
		t.Fatalf("drifts = %+v", sqlc.Drifts)
	}
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("worktree matching base did not select only sqlc")
	}
}

func TestSinceRunsInputRestoredToBase(t *testing.T) {
	root := writeSinceRepo(t)
	testutil.CommitPath(t, root, "queries/q.sql", "select 2;\n")
	if err := os.WriteFile(filepath.Join(root, "queries/q.sql"), []byte("select 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := mustCheckSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("input restored to base did not select only sqlc")
	}
}

func TestSinceRunsWhenConfigCommandChanges(t *testing.T) {
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

	result := mustCheckSince(t, root, "base")
	assertGroupStatus(t, result, "sqlc", check.GroupOK)
	assertGroupStatus(t, result, "protobuf", check.GroupOK)
	if !markerExists(root, "sqlc-cmd") || !markerExists(root, "proto-ran") {
		t.Fatal("config command change did not run groups with inputs")
	}
	got, err := os.ReadFile(filepath.Join(root, "gen", "a.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package gen\n" {
		t.Fatalf("protobuf output = %q", got)
	}
}

func TestSinceCustomConfigChangeRuns(t *testing.T) {
	root, cfg := writeCustomConfigRepo(t)
	path := cfg.Path
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	checked, err := checkSince(cfg, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, checked, "gen", check.GroupOK)
	if !markerExists(root, "ran") {
		t.Fatal("changed custom.yaml did not run check")
	}
	if err := os.Remove(filepath.Join(root, "ran")); err != nil {
		t.Fatal(err)
	}

	ran, err := runSince(cfg, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, ran, "gen", check.GroupOK)
	if !markerExists(root, "ran") {
		t.Fatal("changed custom.yaml did not run")
	}
}

func TestSinceCustomConfigIgnoresUntrackedSibling(t *testing.T) {
	root, cfg := writeCustomConfigRepo(t)
	writeSinceFile(t, root, "genguard.yml", "groups: []\n")

	checked, err := checkSince(cfg, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, checked, "gen", check.GroupSkipped)
	if markerExists(root, "ran") {
		t.Fatal("untracked genguard.yml ran check")
	}

	ran, err := runSince(cfg, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	assertGroupStatus(t, ran, "gen", check.GroupSkipped)
	if markerExists(root, "ran") {
		t.Fatal("untracked genguard.yml ran")
	}
}

func writeCustomConfigRepo(t *testing.T) (string, config.Config) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	writeSinceFile(t, root, "in.txt", "in\n")
	writeSinceFile(t, root, "out.txt", "out\n")
	path, err := testutil.WriteGenguardConfig(root, "", "", "custom.yaml", []testutil.GroupSpec{{
		Name:    "gen",
		Command: `python3 -c "open('ran','w').close()"`,
		Inputs:  []string{"in.txt"},
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func TestSinceRunsUntrackedConfig(t *testing.T) {
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
	result, err := checkSince(cfg, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || result.Groups[0].Status != check.GroupOK {
		t.Fatalf("fresh = %+v", result.Groups)
	}
	if !markerExists(dir, "fresh-ran") {
		t.Fatal("untracked config did not run")
	}
}

func TestSinceRunsMissingLiteralOutput(t *testing.T) {
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

	result := mustCheckSince(t, root, "HEAD")
	sqlc := groupResult(t, result, "sqlc")
	if sqlc.Status != check.GroupDrift {
		t.Fatalf("sqlc status = %q, want drift; err = %v", sqlc.Status, sqlc.Err)
	}
	if len(sqlc.Drifts) != 1 || sqlc.Drifts[0].Kind != "missing" || sqlc.Drifts[0].Path != "internal/db/missing.txt" {
		t.Fatalf("drifts = %+v", sqlc.Drifts)
	}
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") || markerExists(root, "proto-ran") {
		t.Fatal("missing declared output did not select only sqlc")
	}
}

func TestSinceRunsDeletionOfOutputAddedAfterBase(t *testing.T) {
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

	result := mustCheckSince(t, root, "base")
	sqlc := groupResult(t, result, "sqlc")
	if sqlc.Status != check.GroupDrift {
		t.Fatalf("sqlc status = %q, want drift; err = %v", sqlc.Status, sqlc.Err)
	}
	if len(sqlc.Drifts) != 1 || sqlc.Drifts[0].Kind != "missing" || sqlc.Drifts[0].Path != "internal/db/extra.txt" {
		t.Fatalf("drifts = %+v", sqlc.Drifts)
	}
	assertGroupStatus(t, result, "protobuf", check.GroupSkipped)
	if !markerExists(root, "sqlc-ran") {
		t.Fatal("deleted output added after base did not run sqlc")
	}
}

func TestCLISinceAllLabelsSkippedGroups(t *testing.T) {
	root := writeSinceRepo(t)
	testutil.CommitPath(t, root, "queries/q.sql", "select 2;\n")
	testutil.Chdir(t, root)

	stdout, stderr, code := runCLI([]string{"check", "--all", "--since", "base"})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	want := strings.Join([]string{
		"Generated files match the generators.",
		filepath.Join("api", "genguard.yaml"),
		"genguard.yaml",
		"2 configs: 2 ok, 0 drift, 0 error",
		"",
		filepath.Join("api", "genguard.yaml"),
		"  api: skipped",
		"1 group: 0 ok, 0 drift, 0 error, 1 skipped",
		"",
		"genguard.yaml",
		"  sqlc: OK",
		"  protobuf: skipped",
		"  plain: OK",
		"3 groups: 2 ok, 0 drift, 0 error, 1 skipped",
	}, "\n") + "\n"
	if stdout != want {
		t.Fatalf("stdout =\n%s\nwant\n%s", stdout, want)
	}
}

func TestCLIRunAllSinceLabelsSkippedGroups(t *testing.T) {
	root := writeSinceRepo(t)
	testutil.CommitPath(t, root, "queries/q.sql", "select 2;\n")
	testutil.Chdir(t, filepath.Join(root, "api"))

	stdout, stderr, code := runCLI([]string{"run", "--all", "--since", "base"})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	want := strings.Join([]string{
		"Generated files written.",
		filepath.Join("api", "genguard.yaml"),
		"genguard.yaml",
		"2 configs: 2 ok, 0 drift, 0 error",
		"",
		filepath.Join("api", "genguard.yaml"),
		"  api: skipped",
		"1 group: 0 ok, 0 drift, 0 error, 1 skipped",
		"",
		"genguard.yaml",
		"  sqlc: OK",
		"  protobuf: skipped",
		"  plain: OK",
		"3 groups: 2 ok, 0 drift, 0 error, 1 skipped",
	}, "\n") + "\n"
	if stdout != want {
		t.Fatalf("stdout =\n%s\nwant\n%s", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	if !markerExists(root, "sqlc-ran") || !markerExists(root, "plain-ran") || markerExists(root, "proto-ran") || markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("run --all --since did not select per group")
	}
	got, err := os.ReadFile(filepath.Join(root, "gen", "a.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package gen\n" {
		t.Fatalf("clean ran on a skipped group: %q", got)
	}
}

func TestCLIRunAllBadSinceDoesNotRun(t *testing.T) {
	root := writeSinceRepo(t)
	testutil.Chdir(t, root)

	stdout, stderr, code := runCLI([]string{"run", "--all", "--since", "not-a-ref"})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if stdout != "" || !strings.Contains(stderr, "bad --since ref") {
		t.Fatalf("stdout = %q stderr = %q", stdout, stderr)
	}
	if markerExists(root, "plain-ran") || markerExists(root, "sqlc-ran") || markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("bad ref ran a group")
	}
}

func writeSinceRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	writeSinceFile(t, root, ".gitignore", "*.ignore\n")
	writeSinceFile(t, root, "queries/q.sql", "select 1;\n")
	writeSinceFile(t, root, "proto/a.proto", "syntax = \"proto3\";\n")
	writeSinceFile(t, root, "internal/db/out.txt", "db\n")
	writeSinceFile(t, root, "gen/a.pb.go", "package gen\n")
	writeSinceFile(t, root, "plain/out.txt", "plain\n")
	writeSinceFile(t, root, "api/src/a.txt", "api\n")
	writeSinceFile(t, root, "api/out.txt", "out\n")
	if _, err := testutil.WriteGenguardConfig(root, "", "", "", sinceGroups()); err != nil {
		t.Fatal(err)
	}
	apiGroups := []testutil.GroupSpec{{
		Name:    "api",
		Command: `python3 -c "open('api-ran','w').close()"`,
		Inputs:  []string{"src/"},
		Outputs: []string{"out.txt"},
	}}
	if _, err := testutil.WriteGenguardConfig(filepath.Join(root, "api"), "", "", "", apiGroups); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "base"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "branch", "base"); err != nil {
		t.Fatal(err)
	}
	return root
}

func sinceGroups() []testutil.GroupSpec {
	return []testutil.GroupSpec{
		{
			Name:    "sqlc",
			Command: `python3 -c "open('sqlc-ran','w').close()"`,
			Inputs:  []string{"queries/"},
			Outputs: []string{"internal/db/out.txt"},
		},
		{
			Name: "protobuf",
			// Text mode on Windows rewrites LF as CRLF, so the file no longer matches HEAD.
			Command: `python3 -c "open('proto-ran','w').close(); open('gen/a.pb.go','wb').write(b'package gen\n')"`,
			Inputs:  []string{"proto/"},
			Outputs: []string{"gen/a.pb.go"},
			Clean:   true,
		},
		{
			Name:    "plain",
			Command: `python3 -c "open('plain-ran','w').close()"`,
			Outputs: []string{"plain/out.txt"},
		},
	}
}

func writeSinceFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustCheckSince(t *testing.T, root, since string) check.ConfigResult {
	t.Helper()
	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := checkSince(cfg, since)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func groupResult(t *testing.T, result check.ConfigResult, name string) check.GroupResult {
	t.Helper()
	for _, group := range result.Groups {
		if group.Name == name {
			return group
		}
	}
	t.Fatalf("missing group %s", name)
	return check.GroupResult{}
}

func assertGroupStatus(t *testing.T, result check.ConfigResult, name string, status check.GroupStatus) {
	t.Helper()
	group := groupResult(t, result, name)
	if group.Status != status {
		t.Fatalf("%s status = %q, want %q; err = %v", name, group.Status, status, group.Err)
	}
}

func groupInRun(t *testing.T, run check.RunResult, name string) check.GroupResult {
	t.Helper()
	for _, cfg := range run.Configs {
		for _, group := range cfg.Result.Groups {
			if group.Name == name {
				return group
			}
		}
	}
	t.Fatalf("missing group %s", name)
	return check.GroupResult{}
}

func markerExists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}
