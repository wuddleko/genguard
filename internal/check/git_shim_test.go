package check

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("GENGUARD_GIT_SHIM") == "1" {
		os.Exit(gitShimMain())
	}
	os.Unsetenv("GITHUB_WORKSPACE")
	os.Exit(m.Run())
}

func installGitShim(t *testing.T, mode string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if runtime.GOOS == "windows" {
		installWindowsModeShim(t, bin)
		t.Setenv("GENGUARD_GIT_SHIM", "1")
	} else {
		installUnixModeShim(t, bin)
	}
	t.Setenv("GENGUARD_REAL_GIT", real)
	t.Setenv("GENGUARD_GIT_MODE", mode)
	// The shim is the only git on PATH and execs GENGUARD_REAL_GIT by absolute path.
	t.Setenv("PATH", bin)
}

func installUnixModeShim(t *testing.T, bin string) {
	t.Helper()
	script := strings.ReplaceAll(unixGitShim, "@SCRIPT@", shellQuote(filepath.Join(bin, "git")))
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func installWindowsModeShim(t *testing.T, bin string) {
	t.Helper()
	src, err := os.Open(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.OpenFile(filepath.Join(bin, "git.exe"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		t.Fatal(err)
	}
	if err := dst.Close(); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func gitShimMain() int {
	args := os.Args[1:]
	switch os.Getenv("GENGUARD_GIT_MODE") {
	case "exit-2-empty":
		return 2
	case "merge-base-empty":
		if containsArg(args, "merge-base") {
			return 0
		}
	case "merge-base-quiet":
		if containsArg(args, "merge-base") {
			return 2
		}
	case "show-prefix-abs":
		if containsArg(args, "--show-prefix") {
			fmt.Println("/no/such/prefix/")
			return 0
		}
	case "show-prefix-fail":
		if containsArg(args, "--show-prefix") {
			fmt.Fprintln(os.Stderr, "fatal: prefix failed")
			return 128
		}
	case "show-prefix-quiet":
		if containsArg(args, "--show-prefix") {
			return 1
		}
	case "show-toplevel-empty":
		if containsArg(args, "--show-toplevel") {
			return 0
		}
	case "show-toplevel-fail":
		if containsArg(args, "--show-toplevel") {
			fmt.Fprintln(os.Stderr, "fatal: toplevel")
			return 128
		}
	case "others-fail":
		if containsArg(args, "--others") {
			fmt.Fprintln(os.Stderr, "fatal: others")
			return 1
		}
	case "dup-names":
		if containsArg(args, "ls-files") && !containsArg(args, "--others") {
			fmt.Print("dup.go\x00dup.go\x00")
			return 0
		}
		if containsArg(args, "--others") {
			return 0
		}
	case "diff-quiet":
		if containsArg(args, "--no-color") {
			return 129
		}
	case "diff-empty":
		if containsArg(args, "--no-index") {
			return 0
		}
	case "ls-tree-quiet":
		if containsArg(args, "ls-tree") {
			return 2
		}
	case "verify-empty":
		if containsArg(args, "--verify") {
			return 0
		}
	case "verify-quiet":
		if containsArg(args, "--verify") {
			return 2
		}
	case "drop-on-toplevel":
		if containsArg(args, "--show-toplevel") {
			_ = os.Remove(os.Args[0])
		}
	case "drop-after-proxy":
		_ = os.Remove(os.Args[0])
	}
	return execRealGit(args)
}

func execRealGit(args []string) int {
	real := os.Getenv("GENGUARD_REAL_GIT")
	cmd := exec.Command(real, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GENGUARD_GIT_SHIM=") || strings.HasPrefix(entry, "GENGUARD_GIT_MODE=") {
			continue
		}
		env = append(env, entry)
	}
	cmd.Env = env
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		return exitErr.ExitCode()
	}
	fmt.Fprintln(os.Stderr, err.Error())
	return 1
}

const unixGitShim = `#!/bin/sh
real=$GENGUARD_REAL_GIT
mode=$GENGUARD_GIT_MODE
saw_others=0
saw_nocolor=0
saw_noindex=0
saw_merge=0
saw_prefix=0
saw_toplevel=0
saw_ls=0
saw_lstree=0
saw_verify=0
for arg in "$@"; do
  case "$arg" in
    --others) saw_others=1 ;;
    --no-color) saw_nocolor=1 ;;
    --no-index) saw_noindex=1 ;;
    merge-base) saw_merge=1 ;;
    --show-prefix) saw_prefix=1 ;;
    --show-toplevel) saw_toplevel=1 ;;
    ls-files) saw_ls=1 ;;
    ls-tree) saw_lstree=1 ;;
    --verify) saw_verify=1 ;;
  esac
done
case "$mode" in
  exit-2-empty) exit 2 ;;
  merge-base-empty) [ "$saw_merge" -eq 1 ] && exit 0 ;;
  merge-base-quiet) [ "$saw_merge" -eq 1 ] && exit 2 ;;
  show-prefix-abs)
    if [ "$saw_prefix" -eq 1 ]; then printf '%s\n' '/no/such/prefix/'; exit 0; fi
    ;;
  show-prefix-fail)
    if [ "$saw_prefix" -eq 1 ]; then echo 'fatal: prefix failed' >&2; exit 128; fi
    ;;
  show-prefix-quiet) [ "$saw_prefix" -eq 1 ] && exit 1 ;;
  show-toplevel-empty) [ "$saw_toplevel" -eq 1 ] && exit 0 ;;
  show-toplevel-fail)
    if [ "$saw_toplevel" -eq 1 ]; then echo 'fatal: toplevel' >&2; exit 128; fi
    ;;
  others-fail)
    if [ "$saw_others" -eq 1 ]; then echo 'fatal: others' >&2; exit 1; fi
    ;;
  dup-names)
    if [ "$saw_ls" -eq 1 ] && [ "$saw_others" -eq 0 ]; then printf 'dup.go\000dup.go\000'; exit 0; fi
    if [ "$saw_others" -eq 1 ]; then exit 0; fi
    ;;
  diff-quiet) [ "$saw_nocolor" -eq 1 ] && exit 129 ;;
  diff-empty) [ "$saw_noindex" -eq 1 ] && exit 0 ;;
  ls-tree-quiet) [ "$saw_lstree" -eq 1 ] && exit 2 ;;
  verify-empty) [ "$saw_verify" -eq 1 ] && exit 0 ;;
  verify-quiet) [ "$saw_verify" -eq 1 ] && exit 2 ;;
  drop-on-toplevel)
    if [ "$saw_toplevel" -eq 1 ]; then /bin/rm -f @SCRIPT@; fi
    ;;
  drop-after-proxy) /bin/rm -f @SCRIPT@ ;;
esac
exec "$real" "$@"
`
