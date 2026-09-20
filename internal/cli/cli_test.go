package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/regen/internal/cli"
	"github.com/wuddleko/regen/internal/testutil"
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
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("regen\n"), 0o644); err != nil {
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

	stdout, _, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
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
	if !strings.Contains(stderr, "no regen.yaml found") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLIInvalidConfigExit2(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "regen.yaml")
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
	if err := os.WriteFile(filepath.Join(root, "regen.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
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

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
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

	_, stderr, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
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
	configPath, err := testutil.WriteRegenConfig(root, "generated/hello.txt", "", "", nil)
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

	cmd := exec.Command("go", "run", "./cmd/regen", "check", "--config", filepath.Join(root, "regen.yaml"))
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run regen: %v: %s", err, out)
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
	if !strings.Contains(stderr, "1 group: 0 ok, 1 drift, 0 error") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "error: 1 generated path drifted") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "greeting: drift") {
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
		"./cmd/regen", "--version",
	)
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run regen --version: %v: %s", err, out)
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
	if !strings.Contains(stderr, "regen check") {
		t.Fatalf("stderr = %q", stderr)
	}
	if !strings.Contains(stderr, "regen version") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCLICheckHelpExit0(t *testing.T) {
	for _, args := range [][]string{{"check", "-h"}, {"check", "--help"}} {
		_, stderr, code := runCLI(args)
		if code != 0 {
			t.Fatalf("%v: code = %d, want 0; stderr = %q", args, code, stderr)
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
