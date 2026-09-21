package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/cli"
	"github.com/wuddleko/genguard/tests/testutil"
)

func runCLI(args []string) (stdout, stderr string, code int) {
	var outBuf, errBuf bytes.Buffer
	code = cli.RunWithIO(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
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

func TestCLISuccessMessage(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if stdout != "Generated files match the generators.\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestCLINoConfigExit2(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)

	_, stderr, code := runCLI([]string{"check"})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "no genguard.yaml found") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLIInvalidConfigExit2(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	if err := os.WriteFile(configPath, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runCLI([]string{"check", "--config", configPath})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestCLIRunsAllGroupsAfterCommandFailure(t *testing.T) {
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

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "broken: error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "greeting: drift") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "2 groups: 0 ok, 1 drift, 1 error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "error: 1 group failed; 1 group drifted") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLICommandFailureExit2(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	content := "groups:\n" +
		"  - name: greeting\n" +
		"    command: python3 -c \"import sys; sys.stderr.write('line1'+chr(10)+'line2'+chr(10)); sys.exit(3)\"\n" +
		"    outputs:\n" +
		"      - generated/hello.txt\n"
	if err := os.WriteFile(filepath.Join(root, "genguard.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "1 group: 0 ok, 0 drift, 1 error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "[modified]") || strings.Contains(stderr, "[missing]") || strings.Contains(stderr, "[untracked]") {
		t.Fatalf("unexpected drift lines: %q", stderr)
	}
	if strings.Contains(stderr, "diff --git") {
		t.Fatalf("unexpected diff: %q", stderr)
	}
	if !strings.Contains(stderr, "error: command failed") {
		t.Fatalf("stderr = %q", stderr)
	}

	found := false
	for _, line := range strings.Split(stderr, "\n") {
		if !strings.Contains(line, "greeting: error") {
			continue
		}
		found = true
		if !strings.Contains(line, "line1 line2") {
			t.Fatalf("summary line not flattened: %q", line)
		}
	}
	if !found {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLIErrorThenOKGroup(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}},
		{Name: "other", Command: "true", Outputs: []string{"other/out.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	commitPath(t, root, "other/out.txt", "ok\n")

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "broken: error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "other: OK") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "2 groups: 1 ok, 0 drift, 1 error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "[modified]") || strings.Contains(stderr, "[missing]") || strings.Contains(stderr, "[untracked]") {
		t.Fatalf("unexpected drift lines: %q", stderr)
	}
	if strings.Contains(stderr, "diff --git") {
		t.Fatalf("unexpected diff: %q", stderr)
	}
}

func TestCLIErrorOKAndDrift(t *testing.T) {
	groups := []testutil.GroupSpec{
		{Name: "broken", Command: "exit 3", Outputs: []string{"generated/hello.txt"}},
		{Name: "other", Command: "true", Outputs: []string{"other/out.txt"}},
		{Name: "greeting", Command: "python3 scripts/gen.py", Outputs: []string{"generated/hello.txt"}},
	}
	root, err := testutil.MakeRepo(t.TempDir(), "", "", groups)
	if err != nil {
		t.Fatal(err)
	}
	commitPath(t, root, "other/out.txt", "ok\n")
	commitNameChange(t, root)

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "broken: error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "other: OK") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "greeting: drift") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "3 groups: 1 ok, 1 drift, 1 error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "[modified] greeting: generated/hello.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "error: 1 group failed; 1 group drifted") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLINotGitRepoExit2(t *testing.T) {
	root := t.TempDir()
	configPath, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, _, code := runCLI([]string{"check", "--config", configPath})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestCLIAutoDiscoversConfig(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, nested)

	_, _, code := runCLI([]string{"check"})
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
}

func TestCLIEntryPoint(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "run", "./cmd/genguard", "check", "--config", filepath.Join(root, "genguard.yaml"))
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run genguard: %v: %s", err, out)
	}
	if string(out) != "Generated files match the generators.\n" {
		t.Fatalf("stdout = %q", string(out))
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatal(err)
	}
	modPath := strings.TrimSpace(string(out))
	if modPath == "" {
		t.Fatal("GOMOD is empty")
	}
	return filepath.Dir(modPath)
}

func TestCLIDriftExit1(t *testing.T) {
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
	if !strings.Contains(stderr, "1 group: 0 ok, 1 drift, 0 error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "error: 1 generated path drifted") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Summary") || !strings.Contains(stderr, "greeting: drift (1 modified)") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "Drift") || !strings.Contains(stderr, "[modified] greeting: generated/hello.txt") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLIVersion(t *testing.T) {
	previous := cli.Version
	t.Cleanup(func() { cli.Version = previous })
	cli.Version = "1.2.3"

	for _, args := range [][]string{{"version"}, {"--version"}} {
		stdout, stderr, code := runCLI(args)
		if code != 0 {
			t.Fatalf("%v: code = %d, want 0", args, code)
		}
		if stdout != "1.2.3\n" {
			t.Fatalf("%v: stdout = %q", args, stdout)
		}
		if stderr != "" {
			t.Fatalf("%v: stderr = %q", args, stderr)
		}
	}
}

func TestCLIEntryPointVersionLdflags(t *testing.T) {
	cmd := exec.Command(
		"go", "run",
		"-ldflags", "-X main.version=9.9.9",
		"./cmd/genguard", "--version",
	)
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run genguard --version: %v: %s", err, out)
	}
	if string(out) != "9.9.9\n" {
		t.Fatalf("stdout = %q", string(out))
	}
}

func TestCLIHelp(t *testing.T) {
	_, stderr, code := runCLI([]string{"--help"})
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if !strings.Contains(stderr, "genguard check") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "genguard check --all") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "genguard version") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLICheckHelpExit0(t *testing.T) {
	for _, args := range [][]string{{"check", "-h"}, {"check", "--help"}} {
		_, stderr, code := runCLI(args)
		if code != 0 {
			t.Fatalf("%v: code = %d, want 0; stderr = %q", args, code, stderr)
		}
		if !strings.Contains(stderr, "-all") || !strings.Contains(stderr, "genguard.yml") {
			t.Fatalf("%v: stderr = %q", args, stderr)
		}
		if !strings.Contains(stderr, "-config") && !strings.Contains(stderr, "-c") {
			t.Fatalf("%v: stderr = %q", args, stderr)
		}
	}
}

func TestCLICheckUnknownFlagExit2(t *testing.T) {
	_, _, code := runCLI([]string{"check", "--nope"})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestCLIUnknownCommandExit2(t *testing.T) {
	_, _, code := runCLI([]string{"unknown"})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
}

func TestCLICheckAllSuccess(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, filepath.Join(root, "api"), "api", "true")
	writeCLIConfig(t, filepath.Join(root, "web"), "web", "true")
	commitRepo(t, root)
	testutil.Chdir(t, filepath.Join(root, "web"))

	stdout, stderr, code := runCLI([]string{"check", "--all"})
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	want := strings.Join([]string{
		"Generated files match the generators.",
		filepath.Join("api", "genguard.yaml"),
		filepath.Join("web", "genguard.yaml"),
		"2 configs: 2 ok, 0 drift, 0 error",
		"",
	}, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q\nwant %q", stdout, want)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLICheckAllDriftExit1(t *testing.T) {
	root := initCLIRepo(t)
	apiDir := filepath.Join(root, "api")
	writeCLIConfig(t, apiDir, "api", "printf 'new\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(apiDir, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCLIConfig(t, filepath.Join(root, "web"), "web", "true")
	commitRepo(t, root)
	testutil.Chdir(t, filepath.Join(root, "web"))

	stdout, stderr, code := runCLI([]string{"check", "--all"})
	requireCheckAllFailure(t, stdout, stderr, code, 1, []string{
		filepath.Join("api", "genguard.yaml"),
		filepath.Join("web", "genguard.yaml"),
		"web: OK",
		"[modified] api: out.txt",
		"2 configs: 1 ok, 1 drift, 0 error",
		"diff --git",
		"error: 1 generated path drifted; commit the generator output or fix the command",
	}, []string{
		"\nDrift\n" + filepath.Join("web", "genguard.yaml"),
	})
}

func TestCLICheckAllRejectsConfigFlag(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, args := range [][]string{
		{"check", "--all", "-c", "genguard.yaml"},
		{"check", "--all", "--config", "genguard.yaml"},
		{"check", "-c", "genguard.yaml", "--all"},
		{"check", "--config", "genguard.yaml", "--all"},
		{"check", "-all", "-c", "genguard.yaml"},
		{"check", "--all", "-c", "a.yaml", "--config", "b.yaml"},
	} {
		stdout, stderr, code := runCLI(args)
		if code != 2 {
			t.Fatalf("%v: code = %d, want 2; stderr = %q", args, code, stderr)
		}
		if stdout != "" {
			t.Fatalf("%v: stdout = %q", args, stdout)
		}
		if !strings.Contains(stderr, "--all and --config are mutually exclusive") {
			t.Fatalf("%v: stderr = %q", args, stderr)
		}
		if strings.Contains(stderr, "git work tree") {
			t.Fatalf("%v: ran the check: %q", args, stderr)
		}
	}
}

func TestCLICheckAllHelpDoesNotRun(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, args := range [][]string{
		{"check", "--all", "-h"},
		{"check", "--all", "--help"},
		{"check", "-h", "--all"},
	} {
		stdout, stderr, code := runCLI(args)
		if code != 0 {
			t.Fatalf("%v: code = %d, want 0; stderr = %q", args, code, stderr)
		}
		if stdout != "" {
			t.Fatalf("%v: stdout = %q", args, stdout)
		}
		if !strings.Contains(stderr, "-all") || !strings.Contains(stderr, "genguard.yml") {
			t.Fatalf("%v: stderr = %q", args, stderr)
		}
		if strings.Contains(stderr, "git work tree") {
			t.Fatalf("%v: ran the check: %q", args, stderr)
		}
	}
}

func TestCLICheckAllUnknownFlagDoesNotRun(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	stdout, stderr, code := runCLI([]string{"check", "--all", "--nope"})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "nope") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "git work tree") || strings.Contains(stderr, "mutually exclusive") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLICheckAllNoConfigsExit2(t *testing.T) {
	root := initCLIRepo(t)
	testutil.Chdir(t, root)

	stdout, stderr, code := runCLI([]string{"check", "--all"})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "no genguard.yaml or genguard.yml found under repository root") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLICheckAllNotGitRepoExit2(t *testing.T) {
	testutil.Chdir(t, t.TempDir())

	stdout, stderr, code := runCLI([]string{"check", "--all"})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "git work tree") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "no genguard.yaml") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLICheckAllCommandErrorExit2(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, filepath.Join(root, "api"), "api", "exit 3")
	writeCLIConfig(t, filepath.Join(root, "web"), "web", "true")
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllFailure(t, stdout, stderr, code, 2, []string{
		filepath.Join("api", "genguard.yaml"),
		filepath.Join("web", "genguard.yaml"),
		"api: error (command failed (exit 3): no output)",
		"web: OK",
		"2 configs: 1 ok, 0 drift, 1 error",
		"error: command failed (exit 3): no output",
	}, []string{"\nDrift\n", "generated path"})
	requireOrder(t, stderr,
		"api: error (command failed (exit 3): no output)",
		"web: OK",
		"2 configs: 1 ok, 0 drift, 1 error",
		"error: command failed (exit 3): no output",
	)
}

func TestCLICheckAllTwoCommandErrorsExit2(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, filepath.Join(root, "api"), "api", "exit 3")
	writeCLIConfig(t, filepath.Join(root, "web"), "web", "exit 4")
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllFailure(t, stdout, stderr, code, 2, []string{
		"api: error (command failed (exit 3): no output)",
		"web: error (command failed (exit 4): no output)",
		"2 configs: 0 ok, 0 drift, 2 error",
		"error: 2 configs failed",
	}, []string{"\nDrift\n", "generated path", ": OK"})
}

func TestCLICheckAllInvalidYAMLStillRunsOthers(t *testing.T) {
	root := initCLIRepo(t)
	apiDir := filepath.Join(root, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "genguard.yaml"), []byte("groups: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCLIConfig(t, filepath.Join(root, "web"), "web", "true")
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllFailure(t, stdout, stderr, code, 2, []string{
		filepath.Join("api", "genguard.yaml"),
		"  error (parse ",
		"web: OK",
		"2 configs: 1 ok, 0 drift, 1 error",
		"error: parse ",
	}, []string{"\nDrift\n", "generated path"})
}

func TestCLICheckAllDriftAndErrorExit2(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, filepath.Join(root, "api"), "api", "exit 3")
	webDir := filepath.Join(root, "web")
	writeCLIConfig(t, webDir, "web", "printf 'new\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(webDir, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllFailure(t, stdout, stderr, code, 2, []string{
		"api: error (command failed (exit 3): no output)",
		"web: drift (1 modified)",
		"[modified] web: out.txt",
		"diff --git",
		"2 configs: 0 ok, 1 drift, 1 error",
		"error: 1 config failed; 1 config drifted",
	}, []string{"[modified] api:", "generated path"})
	requireOrder(t, stderr,
		"api: error (command failed (exit 3): no output)",
		"web: drift (1 modified)",
		"2 configs: 0 ok, 1 drift, 1 error",
		"\nDrift\n",
		"[modified] web: out.txt",
		"error: 1 config failed; 1 config drifted",
	)
}

func TestCLICheckAllTwoDriftsExit1(t *testing.T) {
	root := initCLIRepo(t)
	apiDir := filepath.Join(root, "api")
	webDir := filepath.Join(root, "web")
	writeCLIConfig(t, apiDir, "api", "printf 'aaa\\n' > out.txt")
	writeCLIConfig(t, webDir, "web", "printf 'bbb\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(apiDir, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllFailure(t, stdout, stderr, code, 1, []string{
		"api: drift (1 modified)",
		"web: drift (1 modified)",
		"[modified] api: out.txt",
		"[modified] web: out.txt",
		"+aaa",
		"+bbb",
		"2 configs: 0 ok, 2 drift, 0 error",
		"error: 2 generated paths drifted; commit the generator output or fix the command",
	}, []string{": OK", ": error"})
	requireOrder(t, stderr,
		"[modified] api: out.txt",
		"+aaa",
		"[modified] web: out.txt",
		"+bbb",
		"error: 2 generated paths drifted",
	)
}

func TestCLICheckAllMissingAndUntrackedExit1(t *testing.T) {
	root := initCLIRepo(t)
	apiDir := filepath.Join(root, "api")
	webDir := filepath.Join(root, "web")
	writeCLIConfig(t, apiDir, "api", "true")
	writeNamedCLIConfig(t, webDir, "genguard.yaml", "web", "true", []string{"extra.txt"}, false)
	commitRepo(t, root)
	if err := os.Remove(filepath.Join(apiDir, "out.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "extra.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllFailure(t, stdout, stderr, code, 1, []string{
		"api: drift (1 missing)",
		"web: drift (1 untracked)",
		"[missing] api: out.txt",
		"[untracked] web: extra.txt",
		"2 configs: 0 ok, 2 drift, 0 error",
		"error: 2 generated paths drifted; commit the generator output or fix the command",
	}, nil)
	requireOrder(t, stderr,
		"[missing] api: out.txt",
		"[untracked] web: extra.txt",
	)
}

func TestCLICheckAllDiffFailureExit2(t *testing.T) {
	root := initCLIRepo(t)
	apiDir := filepath.Join(root, "api")
	webDir := filepath.Join(root, "web")
	writeCLIConfig(t, apiDir, "api", "printf 'aaa\\n' > out.txt")
	writeCLIConfig(t, webDir, "web", "printf 'bbb\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(apiDir, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitRepo(t, root)
	failGitDiffIn(t, string(filepath.Separator)+"web")

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllFailure(t, stdout, stderr, code, 2, []string{
		"api: drift (1 modified)",
		"web: drift (1 modified)",
		"[modified] api: out.txt",
		"+aaa",
		"[modified] web: out.txt",
		"error: forced diff failure",
	}, []string{"+bbb", "generated path"})
	requireOrder(t, stderr,
		"[modified] api: out.txt",
		"+aaa",
		"[modified] web: out.txt",
		"error: forced diff failure",
	)
}

func TestCLICheckAllDiscoversYml(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, filepath.Join(root, "api"), "api", "true")
	writeNamedCLIConfig(t, filepath.Join(root, "web"), "genguard.yml", "web", "true", []string{"out.txt"}, false)
	if err := os.WriteFile(filepath.Join(root, "web", "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, filepath.Join(root, "web"))
	requireCheckAllSuccess(t, stdout, stderr, code,
		filepath.Join("api", "genguard.yaml"),
		filepath.Join("web", "genguard.yml"),
	)
}

func TestCLICheckAllYmlOnly(t *testing.T) {
	root := initCLIRepo(t)
	writeNamedCLIConfig(t, root, "genguard.yml", "root", "true", []string{"out.txt"}, false)
	if err := os.WriteFile(filepath.Join(root, "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllSuccess(t, stdout, stderr, code, "genguard.yml")
}

func TestCLICheckAllSameDirectoryYamlAndYml(t *testing.T) {
	root := initCLIRepo(t)
	dir := filepath.Join(root, "api")
	writeCLIConfig(t, dir, "yaml", "true")
	writeNamedCLIConfig(t, dir, "genguard.yml", "yml", "true", []string{"out.txt"}, false)
	writeCLIConfig(t, filepath.Join(root, "web"), "web", "true")
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	if code != 2 {
		t.Fatalf("code = %d, want 2; stdout = %q stderr = %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "both genguard.yaml and genguard.yml") || !strings.Contains(stderr, dir) {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stdout, "Generated files match") || strings.Contains(stderr, "web") {
		t.Fatalf("other configs should not run\nstdout = %q\nstderr = %q", stdout, stderr)
	}
}

func TestCLICheckRejectsBothNames(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, root, "yaml", "true")
	writeNamedCLIConfig(t, root, "genguard.yml", "yml", "true", []string{"out.txt"}, false)
	commitRepo(t, root)
	testutil.Chdir(t, root)

	stdout, stderr, code := runCLI([]string{"check"})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stdout = %q stderr = %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "both genguard.yaml and genguard.yml") || !strings.Contains(stderr, root) {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLIConfigFlagSelectsOneOfBothNames(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, root, "yaml", "true")
	writeNamedCLIConfig(t, root, "genguard.yml", "yml", "exit 3", []string{"out.txt"}, false)
	commitRepo(t, root)

	stdout, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "genguard.yaml")})
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if stdout != "Generated files match the generators.\n" {
		t.Fatalf("stdout = %q", stdout)
	}
}

func TestCLICheckAllNestedFromDeepest(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, filepath.Join(root, "api"), "api", "true")
	writeCLIConfig(t, filepath.Join(root, "api", "proto"), "proto", "true")
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, filepath.Join(root, "api", "proto"))
	requireCheckAllSuccess(t, stdout, stderr, code,
		filepath.Join("api", "genguard.yaml"),
		filepath.Join("api", "proto", "genguard.yaml"),
	)
}

func TestCLICheckAllSingleConfigFromEmptyDir(t *testing.T) {
	root := initCLIRepo(t)
	writeCLIConfig(t, root, "root", "true")
	commitRepo(t, root)
	empty := filepath.Join(root, "services", "worker")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, empty)

	for _, args := range [][]string{{"check", "--all"}, {"check", "-all"}} {
		stdout, stderr, code := runCLI(args)
		requireCheckAllSuccess(t, stdout, stderr, code, "genguard.yaml")
	}
}

func TestCLICheckAllSkipsVendorGitNodeModules(t *testing.T) {
	root := initCLIRepo(t)
	for _, dir := range []string{
		filepath.Join(root, ".git", "hooks"),
		filepath.Join(root, "vendor", "lib"),
		filepath.Join(root, "web", "node_modules", "pkg"),
	} {
		writeCLIConfig(t, dir, "hidden", "true")
	}
	writeCLIConfig(t, filepath.Join(root, "api"), "api", "true")
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllSuccess(t, stdout, stderr, code, filepath.Join("api", "genguard.yaml"))
}

func TestCLICheckAllCleanRefusalStillRunsOther(t *testing.T) {
	root := initCLIRepo(t)
	writeNamedCLIConfig(t, filepath.Join(root, "api"), "genguard.yaml", "api", "true", []string{"."}, true)
	writeCLIConfig(t, filepath.Join(root, "web"), "web", "true")
	commitRepo(t, root)

	stdout, stderr, code := cliCheckAll(t, root)
	requireCheckAllFailure(t, stdout, stderr, code, 2, []string{
		`api: error (clean refuses "."`,
		"web: OK",
		"2 configs: 1 ok, 0 drift, 1 error",
		`error: clean refuses "."`,
	}, []string{"\nDrift\n"})
}

func TestCLICheckAllDiscoveryErrorExit2(t *testing.T) {
	root := initCLIRepo(t)
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	if f, err := os.Open(blocked); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced")
	}
	testutil.Chdir(t, root)

	stdout, stderr, code := runCLI([]string{"check", "--all"})
	if code != 2 {
		t.Fatalf("code = %d, want 2; stderr = %q", code, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "error:") {
		t.Fatalf("stderr = %q", stderr)
	}
	if strings.Contains(stderr, "no genguard.yaml or genguard.yml found") {
		t.Fatalf("walk error treated as empty discovery:\n%s", stderr)
	}
}

func initCLIRepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeCLIConfig(t *testing.T, dir, name, command string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(dir, "", "", "", []testutil.GroupSpec{{
		Name:    name,
		Command: command,
		Outputs: []string{"out.txt"},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitRepo(t *testing.T, root string) {
	t.Helper()
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
}

func writeNamedCLIConfig(t *testing.T, dir, fileName, group, command string, outputs []string, clean bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(dir, "", "", fileName, []testutil.GroupSpec{{
		Name:    group,
		Command: command,
		Outputs: outputs,
		Clean:   clean,
	}}); err != nil {
		t.Fatal(err)
	}
}

func cliCheckAll(t *testing.T, dir string) (string, string, int) {
	t.Helper()
	empty := filepath.Join(dir, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, empty)
	return runCLI([]string{"check", "--all"})
}

func requireCheckAllSuccess(t *testing.T, stdout, stderr string, code int, paths ...string) {
	t.Helper()
	if code != 0 {
		t.Fatalf("code = %d, want 0; stderr = %q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q", stderr)
	}
	noun := "configs"
	if len(paths) == 1 {
		noun = "config"
	}
	totals := strconv.Itoa(len(paths)) + " " + noun + ": " + strconv.Itoa(len(paths)) + " ok, 0 drift, 0 error"
	lines := append([]string{"Generated files match the generators."}, paths...)
	lines = append(lines, totals, "")
	want := strings.Join(lines, "\n")
	if stdout != want {
		t.Fatalf("stdout = %q\nwant %q", stdout, want)
	}
}

func requireCheckAllFailure(t *testing.T, stdout, stderr string, code, want int, present, absent []string) {
	t.Helper()
	if code != want {
		t.Fatalf("code = %d, want %d\nstdout = %q\nstderr:\n%s", code, want, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty\nstderr:\n%s", stdout, stderr)
	}
	for _, part := range present {
		if !strings.Contains(stderr, part) {
			t.Fatalf("stderr missing %q:\n%s", part, stderr)
		}
	}
	for _, part := range absent {
		if part != "" && strings.Contains(stderr, part) {
			t.Fatalf("stderr unexpectedly contains %q:\n%s", part, stderr)
		}
	}
}

func requireOrder(t *testing.T, s string, parts ...string) {
	t.Helper()
	prev := -1
	for _, part := range parts {
		i := strings.Index(s, part)
		if i < 0 || i <= prev {
			t.Fatalf("order missing %q after index %d:\n%s", part, prev, s)
		}
		prev = i
	}
}

// failGitDiffIn makes git diff --no-color fail when -C names a directory
// ending in suffix. Drift detection does not pass --no-color, so the check
// still records drift and the CLI hits the report-error exit.
func failGitDiffIn(t *testing.T, suffix string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		"root=\nprev=\nnocolor=0\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"-C\" ]; then\n" +
		"    root=$arg\n" +
		"  fi\n" +
		"  if [ \"$arg\" = \"--no-color\" ]; then\n" +
		"    nocolor=1\n" +
		"  fi\n" +
		"  prev=$arg\n" +
		"done\n" +
		"if [ \"$nocolor\" -eq 1 ]; then\n" +
		"  case \"$root\" in\n" +
		"    *" + suffix + ")\n" +
		"      echo \"forced diff failure\" >&2\n" +
		"      exit 129\n" +
		"      ;;\n" +
		"  esac\n" +
		"fi\n" +
		"exec " + shellQuote(real) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
