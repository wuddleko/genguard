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
	script := strings.ReplaceAll(unixGitShim(), "@SCRIPT@", shellQuote(filepath.Join(bin, "git")))
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

// gitShimModes is the mode list for both shims. Unix tests run a shell script
// generated from it. Windows tests run gitShimMain.
var gitShimModes = []shimMode{
	{name: "exit-2-empty", steps: []shimStep{shimExit(2)}},
	{name: "merge-base-empty", steps: []shimStep{shimExit(0, "merge-base")}},
	{name: "merge-base-quiet", steps: []shimStep{shimExit(2, "merge-base")}},
	{name: "show-prefix-abs", steps: []shimStep{{
		when: shimOn("--show-prefix"), stdout: "/no/such/prefix/\n", stop: true,
	}}},
	{name: "show-prefix-fail", steps: []shimStep{{
		when: shimOn("--show-prefix"), stderr: "fatal: prefix failed\n", code: 128, stop: true,
	}}},
	{name: "show-prefix-quiet", steps: []shimStep{shimExit(1, "--show-prefix")}},
	{name: "show-toplevel-empty", steps: []shimStep{shimExit(0, "--show-toplevel")}},
	{name: "show-toplevel-fail", steps: []shimStep{{
		when: shimOn("--show-toplevel"), stderr: "fatal: toplevel\n", code: 128, stop: true,
	}}},
	{name: "others-fail", steps: []shimStep{{
		when: shimOn("--others"), stderr: "fatal: others\n", code: 1, stop: true,
	}}},
	{name: "dup-names", steps: []shimStep{
		{
			when:   &shimWhen{all: []string{"ls-files"}, none: []string{"--others"}},
			stdout: "dup.go\x00dup.go\x00",
			stop:   true,
		},
		{when: shimOn("--others"), stop: true},
	}},
	{name: "diff-quiet", steps: []shimStep{shimExit(129, "--no-color")}},
	{name: "diff-empty", steps: []shimStep{shimExit(0, "--no-index")}},
	{name: "ls-tree-quiet", steps: []shimStep{shimExit(2, "ls-tree")}},
	{name: "verify-empty", steps: []shimStep{shimExit(0, "--verify")}},
	{name: "verify-quiet", steps: []shimStep{shimExit(2, "--verify")}},
	{name: "drop-on-toplevel", steps: []shimStep{{when: shimOn("--show-toplevel"), drop: true}}},
	{name: "drop-after-proxy", steps: []shimStep{{drop: true}}},
}

type shimWhen struct {
	all  []string
	none []string
}

func (w *shimWhen) match(args []string) bool {
	if w == nil {
		return true
	}
	for _, arg := range w.all {
		if !containsArg(args, arg) {
			return false
		}
	}
	for _, arg := range w.none {
		if containsArg(args, arg) {
			return false
		}
	}
	return true
}

func shimOn(args ...string) *shimWhen {
	return &shimWhen{all: args}
}

type shimStep struct {
	when   *shimWhen
	stdout string
	stderr string
	code   int
	stop   bool
	drop   bool
}

func shimExit(code int, args ...string) shimStep {
	step := shimStep{code: code, stop: true}
	if len(args) > 0 {
		step.when = shimOn(args...)
	}
	return step
}

type shimMode struct {
	name  string
	steps []shimStep
}

func gitShimMain() int {
	args := os.Args[1:]
	mode := os.Getenv("GENGUARD_GIT_MODE")
	for _, spec := range gitShimModes {
		if spec.name != mode {
			continue
		}
		for _, step := range spec.steps {
			if !step.when.match(args) {
				continue
			}
			if step.drop {
				dropShim()
			}
			if step.stdout != "" {
				fmt.Print(step.stdout)
			}
			if step.stderr != "" {
				fmt.Fprint(os.Stderr, step.stderr)
			}
			if step.stop {
				return step.code
			}
		}
		break
	}
	return execRealGit(args)
}

func dropShim() {
	path, err := os.Executable()
	if err != nil {
		path = os.Args[0]
	}
	if os.Remove(path) == nil {
		return
	}
	// Windows will not delete a running executable. Renaming it off PATH
	// makes the next git lookup fail.
	_ = os.Rename(path, path+".dropped")
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

func unixGitShim() string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("real=$GENGUARD_REAL_GIT\n")
	b.WriteString("mode=$GENGUARD_GIT_MODE\n")
	watched := shimWatchedArgs()
	for _, arg := range watched {
		fmt.Fprintf(&b, "%s=0\n", shellVar(arg))
	}
	b.WriteString("for arg in \"$@\"; do\n")
	b.WriteString("  case \"$arg\" in\n")
	for _, arg := range watched {
		fmt.Fprintf(&b, "    %s) %s=1 ;;\n", arg, shellVar(arg))
	}
	b.WriteString("  esac\ndone\n")
	b.WriteString("case \"$mode\" in\n")
	for _, mode := range gitShimModes {
		fmt.Fprintf(&b, "  %s)\n", mode.name)
		for _, step := range mode.steps {
			writeShimStep(&b, step)
		}
		b.WriteString("    ;;\n")
	}
	b.WriteString("esac\nexec \"$real\" \"$@\"\n")
	return b.String()
}

func shimWatchedArgs() []string {
	seen := map[string]bool{}
	var args []string
	add := func(arg string) {
		if seen[arg] {
			return
		}
		seen[arg] = true
		args = append(args, arg)
	}
	for _, mode := range gitShimModes {
		for _, step := range mode.steps {
			if step.when == nil {
				continue
			}
			for _, arg := range step.when.all {
				add(arg)
			}
			for _, arg := range step.when.none {
				add(arg)
			}
		}
	}
	return args
}

func shellVar(arg string) string {
	name := strings.TrimLeft(arg, "-")
	name = strings.ReplaceAll(name, "-", "_")
	return "saw_" + name
}

func writeShimStep(b *strings.Builder, step shimStep) {
	cond := shellCond(step.when)
	indent := "    "
	if cond != "" {
		fmt.Fprintf(b, "    if %s; then\n", cond)
		indent = "      "
	}
	if step.stdout != "" {
		fmt.Fprintf(b, "%s%s\n", indent, shellPrintf(step.stdout, ""))
	}
	if step.stderr != "" {
		fmt.Fprintf(b, "%s%s\n", indent, shellPrintf(step.stderr, ">&2"))
	}
	if step.drop {
		fmt.Fprintf(b, "%s/bin/rm -f @SCRIPT@\n", indent)
	}
	if step.stop {
		fmt.Fprintf(b, "%sexit %d\n", indent, step.code)
	}
	if cond != "" {
		b.WriteString("    fi\n")
	}
}

func shellCond(w *shimWhen) string {
	if w == nil {
		return ""
	}
	parts := make([]string, 0, len(w.all)+len(w.none))
	for _, arg := range w.all {
		parts = append(parts, fmt.Sprintf("[ \"$%s\" -eq 1 ]", shellVar(arg)))
	}
	for _, arg := range w.none {
		parts = append(parts, fmt.Sprintf("[ \"$%s\" -eq 0 ]", shellVar(arg)))
	}
	return strings.Join(parts, " && ")
}

func shellPrintf(data, redirect string) string {
	var b strings.Builder
	b.WriteString("printf '")
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '\n':
			b.WriteString(`\n`)
		case 0:
			b.WriteString(`\000`)
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`'\''`)
		default:
			b.WriteByte(data[i])
		}
	}
	b.WriteByte('\'')
	if redirect != "" {
		b.WriteByte(' ')
		b.WriteString(redirect)
	}
	return b.String()
}
