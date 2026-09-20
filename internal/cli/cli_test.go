package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/regen-check/regen/internal/cli"
	"github.com/regen-check/regen/internal/testutil"
)

func runCLI(args []string) (stdout, stderr string, code int) {
	var outBuf, errBuf bytes.Buffer
	code = cli.RunWithIO(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
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

func TestCLICommandFailureExit2(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	content := "groups:\n" +
		"  - name: greeting\n" +
		"    command: python3 -c \"import sys; sys.exit(3)\"\n" +
		"    outputs:\n" +
		"      - generated/hello.txt\n"
	if err := os.WriteFile(filepath.Join(root, "regen.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, code := runCLI([]string{"check", "--config", filepath.Join(root, "regen.yaml")})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
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
	if !strings.Contains(stderr, "error: 1 generated path(s) drifted") {
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
