package action_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestActionScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("action.sh fixtures are shell scripts")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	script := repoFile(t, "action.sh")
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExe(t, filepath.Join(bin, "genguard"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$GENGUARD_TEST_ARGS\"\nexit \"${GENGUARD_TEST_CODE:-0}\"\n")
	writeExe(t, filepath.Join(bin, "go"), "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$GENGUARD_TEST_GO\"\nif [ \"${1:-}\" = env ]; then\n  case \"${2:-}\" in\n    GOBIN) printf '%s\\n' \"${GOBIN:-}\" ;;\n    GOPATH) printf '%s\\n' \"${GOPATH:-${HOME:-}/go}\" ;;\n  esac\nfi\n")
	writeExe(t, filepath.Join(root, "install.sh"), "#!/bin/sh\nif [ -z \"${BINDIR:-}\" ]; then\n  printf 'BINDIR unset\\n' >&2\n  exit 1\nfi\nfirst=${PATH%%:*}\nif [ \"$first\" != \"$BINDIR\" ]; then\n  printf 'BINDIR is not first on PATH: %s\\n' \"$PATH\" >&2\n  exit 1\nfi\nprintf '%s\\n' \"$1\" > \"$GENGUARD_TEST_INSTALL\"\nprintf '%s\\n' \"$BINDIR\" > \"$GENGUARD_TEST_BINDIR\"\n")

	t.Run("release tag", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out")
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		pathFile := filepath.Join(out, "path")
		runAction(t, script, bin, root, out, []string{"install"}, []string{
			"GENGUARD_ACTION_REF=v0.5.0",
			"GITHUB_PATH=" + pathFile,
		})
		if got := readFile(t, filepath.Join(out, "install")); got != "v0.5.0\n" {
			t.Fatalf("install tag = %q", got)
		}
		bindir := filepath.Join(out, "genguard-bin")
		if got := readFile(t, filepath.Join(out, "bindir")); got != bindir+"\n" {
			t.Fatalf("BINDIR = %q", got)
		}
		if got := readFile(t, pathFile); got != bindir+"\n" {
			t.Fatalf("GITHUB_PATH = %q", got)
		}
		if _, err := os.Stat(filepath.Join(out, "go")); err == nil {
			t.Fatal("release tag built from source")
		}
	})

	t.Run("refs/tags prefix", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out")
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		runAction(t, script, bin, root, out, []string{"install"}, []string{"GENGUARD_ACTION_REF=refs/tags/v0.5.0"})
		if got := readFile(t, filepath.Join(out, "install")); got != "v0.5.0\n" {
			t.Fatalf("install tag = %q", got)
		}
		if got := readFile(t, filepath.Join(out, "bindir")); got != filepath.Join(out, "genguard-bin")+"\n" {
			t.Fatalf("BINDIR = %q", got)
		}
	})

	t.Run("runner action ref is not a release", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out")
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		runAction(t, script, bin, root, out, []string{"install"}, []string{"GITHUB_ACTION_REF=refs/tags/v0.5.0"})
		if _, err := os.Stat(filepath.Join(out, "install")); err == nil {
			t.Fatal("GITHUB_ACTION_REF downloaded a release")
		}
		got := readFile(t, filepath.Join(out, "go"))
		if !strings.HasPrefix(got, "-C\n"+root+"\ninstall\n-mod=mod\n./cmd/genguard\n") {
			t.Fatalf("go args = %q", got)
		}
	})

	t.Run("incomplete tag is rejected", func(t *testing.T) {
		for _, ref := range []string{"v1", "refs/tags/v1.2"} {
			t.Run(ref, func(t *testing.T) {
				out := filepath.Join(t.TempDir(), "out")
				if err := os.MkdirAll(out, 0o755); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("sh", append([]string{script}, "install")...)
				cmd.Env = []string{
					"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
					"GITHUB_ACTION_PATH=" + root,
					"GENGUARD_ACTION_REF=" + ref,
					"GENGUARD_TEST_ARGS=" + filepath.Join(out, "args"),
					"GENGUARD_TEST_GO=" + filepath.Join(out, "go"),
					"GENGUARD_TEST_INSTALL=" + filepath.Join(out, "install"),
					"GENGUARD_TEST_BINDIR=" + filepath.Join(out, "bindir"),
					"RUNNER_TEMP=" + out,
				}
				output, err := cmd.CombinedOutput()
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ExitCode() != 2 {
					t.Fatalf("exit %v, output %q", err, output)
				}
				if !strings.Contains(string(output), "is not a release tag (want v1.2.3)") {
					t.Fatalf("output = %q", output)
				}
				if _, err := os.Stat(filepath.Join(out, "install")); err == nil {
					t.Fatal("incomplete tag downloaded a release")
				}
				if _, err := os.Stat(filepath.Join(out, "go")); err == nil {
					t.Fatal("incomplete tag built from source")
				}
			})
		}
	})

	t.Run("local ref builds the checkout", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out")
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		gobin := filepath.Join(out, "gobin")
		pathFile := filepath.Join(out, "path")
		runAction(t, script, bin, root, out, []string{"install"}, []string{
			"GENGUARD_ACTION_REF=refs/heads/main",
			"GOBIN=" + gobin,
			"GITHUB_PATH=" + pathFile,
		})
		if _, err := os.Stat(filepath.Join(out, "install")); err == nil {
			t.Fatal("branch ref downloaded a release")
		}
		got := readFile(t, filepath.Join(out, "go"))
		if !strings.HasPrefix(got, "-C\n"+root+"\ninstall\n-mod=mod\n./cmd/genguard\n") {
			t.Fatalf("go args = %q", got)
		}
		if got := readFile(t, pathFile); got != gobin+"\n" {
			t.Fatalf("GITHUB_PATH = %q", got)
		}
	})

	t.Run("check forwards flags", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "out")
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		runAction(t, script, bin, root, out, []string{"check"}, []string{
			"GENGUARD_ALL=true",
			"GENGUARD_CONFIG=my config.yaml",
			"GENGUARD_SINCE=origin/main",
			"GENGUARD_ISOLATED=true",
			"GENGUARD_TEST_CODE=1",
		})
		got := readFile(t, filepath.Join(out, "args"))
		want := "check\n--all\n--config\nmy config.yaml\n--since\norigin/main\n--isolated\n"
		if got != want {
			t.Fatalf("args = %q", got)
		}
	})

	t.Run("usage", func(t *testing.T) {
		cmd := exec.Command("sh", script)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected failure, got %q", out)
		}
		if !strings.Contains(string(out), "usage:") {
			t.Fatalf("output = %q", out)
		}
	})
}

func runAction(t *testing.T, script, bin, actionPath, out string, args, extra []string) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	env := []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"GITHUB_ACTION_PATH=" + actionPath,
		"GENGUARD_TEST_ARGS=" + filepath.Join(out, "args"),
		"GENGUARD_TEST_GO=" + filepath.Join(out, "go"),
		"GENGUARD_TEST_INSTALL=" + filepath.Join(out, "install"),
		"GENGUARD_TEST_BINDIR=" + filepath.Join(out, "bindir"),
		"RUNNER_TEMP=" + out,
	}
	cmd.Env = append(env, extra...)
	outBytes, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = exit.ExitCode()
	}
	want := 0
	for _, e := range extra {
		if e == "GENGUARD_TEST_CODE=1" {
			want = 1
		}
	}
	if code != want {
		t.Fatalf("exit %d, want %d: %s", code, want, outBytes)
	}
}

func writeExe(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func repoFile(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", name)
}
