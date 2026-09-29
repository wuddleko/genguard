package command

import (
	"errors"
	"testing"
)

func TestShellInvocationUnix(t *testing.T) {
	t.Parallel()
	name, args, cmdLine := shellInvocationFor("linux", nil, "", "buf generate")
	if name != "sh" {
		t.Fatalf("name = %q, want sh", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "buf generate" {
		t.Fatalf("args = %q", args)
	}
	if cmdLine != "" {
		t.Fatalf("cmdline = %q", cmdLine)
	}
}

func TestShellInvocationWindowsPrefersSh(t *testing.T) {
	t.Parallel()
	look := func(n string) (string, error) {
		switch n {
		case "sh":
			return `C:\Git\bin\sh.exe`, nil
		case "git":
			return `C:\Program Files\Git\cmd\git.exe`, nil
		case `C:\Program Files\Git\bin\bash.exe`:
			return `C:\Program Files\Git\bin\bash.exe`, nil
		default:
			return "", errors.New("not found")
		}
	}
	name, args, cmdLine := shellInvocationFor("windows", look, `C:\Windows\system32\cmd.exe`, "buf generate")
	if name != `C:\Git\bin\sh.exe` {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "buf generate" {
		t.Fatalf("args = %q", args)
	}
	if cmdLine != "" {
		t.Fatalf("cmdline = %q", cmdLine)
	}
}

func TestShellInvocationWindowsPrefersGitBash(t *testing.T) {
	t.Parallel()
	const bash = `C:\Program Files\Git\bin\bash.exe`
	look := func(n string) (string, error) {
		switch n {
		case "git":
			return `C:\Program Files\Git\cmd\git.exe`, nil
		case bash:
			return bash, nil
		case "bash":
			return `C:\Windows\System32\bash.exe`, nil
		default:
			return "", errors.New("not found")
		}
	}
	name, args, cmdLine := shellInvocationFor("windows", look, `C:\Windows\system32\cmd.exe`, "make generate")
	if name != bash {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "make generate" {
		t.Fatalf("args = %q", args)
	}
	if cmdLine != "" {
		t.Fatalf("cmdline = %q", cmdLine)
	}
}

func TestShellInvocationWindowsPrefersBashIfNoSh(t *testing.T) {
	t.Parallel()
	look := func(n string) (string, error) {
		if n == "bash" {
			return `C:\Git\bin\bash.exe`, nil
		}
		return "", errors.New("not found")
	}
	name, args, cmdLine := shellInvocationFor("windows", look, `C:\Windows\system32\cmd.exe`, "make generate")
	if name != `C:\Git\bin\bash.exe` {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "make generate" {
		t.Fatalf("args = %q", args)
	}
	if cmdLine != "" {
		t.Fatalf("cmdline = %q", cmdLine)
	}
}

func TestShellInvocationWindowsSkipsSystem32Bash(t *testing.T) {
	t.Parallel()
	look := func(n string) (string, error) {
		if n == "sh" || n == "bash" {
			return `C:\Windows\System32\bash.exe`, nil
		}
		return "", errors.New("not found")
	}
	name, args, cmdLine := shellInvocationFor("windows", look, `C:\Windows\system32\cmd.exe`, "echo hi")
	if name != `C:\Windows\system32\cmd.exe` {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 0 {
		t.Fatalf("args = %q", args)
	}
	if cmdLine != `C:\Windows\system32\cmd.exe /C echo hi` {
		t.Fatalf("cmdline = %q", cmdLine)
	}
}

func TestShellInvocationWindowsFallsBackToComspec(t *testing.T) {
	t.Parallel()
	look := func(string) (string, error) {
		return "", errors.New("not found")
	}
	name, args, cmdLine := shellInvocationFor("windows", look, `D:\cmd.exe`, "buf generate")
	if name != `D:\cmd.exe` {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 0 {
		t.Fatalf("args = %q", args)
	}
	if cmdLine != `D:\cmd.exe /C buf generate` {
		t.Fatalf("cmdline = %q", cmdLine)
	}
}

func TestShellInvocationWindowsDefaultCmd(t *testing.T) {
	t.Parallel()
	look := func(string) (string, error) {
		return "", errors.New("not found")
	}
	name, args, cmdLine := shellInvocationFor("windows", look, "  ", "true")
	if name != "cmd.exe" {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 0 {
		t.Fatalf("args = %q", args)
	}
	if cmdLine != `cmd.exe /C true` {
		t.Fatalf("cmdline = %q", cmdLine)
	}
}

func TestShellInvocationWindowsCmdLineKeepsSpaces(t *testing.T) {
	t.Parallel()
	look := func(string) (string, error) {
		return "", errors.New("not found")
	}
	command := `python3 "C:\Users\A B\nap.py"`
	name, args, cmdLine := shellInvocationFor("windows", look, `C:\Windows\system32\cmd.exe`, command)
	if name != `C:\Windows\system32\cmd.exe` {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 0 {
		t.Fatalf("args = %q", args)
	}
	want := `C:\Windows\system32\cmd.exe /C python3 "C:\Users\A B\nap.py"`
	if cmdLine != want {
		t.Fatalf("cmdline = %q", cmdLine)
	}
}

func TestQuoteForInvocation(t *testing.T) {
	t.Parallel()
	miss := func(string) (string, error) {
		return "", errors.New("not found")
	}
	_, cmdArgs, cmdLine := shellInvocationFor("windows", miss, `C:\Windows\system32\cmd.exe`, "")
	got := quoteForInvocation(cmdArgs, cmdLine, `C:\Users\A B\100% "nap".py`)
	if got != `"C:\Users\A B\100%% ""nap"".py"` {
		t.Fatalf("cmd = %q", got)
	}

	lookSh := func(n string) (string, error) {
		if n == "sh" {
			return `C:\Git\bin\sh.exe`, nil
		}
		return "", errors.New("not found")
	}
	_, shArgs, shLine := shellInvocationFor("windows", lookSh, `cmd.exe`, "")
	got = quoteForInvocation(shArgs, shLine, `C:\Users\A B\nap.py`)
	if got != `'C:\Users\A B\nap.py'` {
		t.Fatalf("sh = %q", got)
	}

	_, unixArgs, unixLine := shellInvocationFor("linux", nil, "", "")
	got = quoteForInvocation(unixArgs, unixLine, "it's")
	if got != `'it'\''s'` {
		t.Fatalf("unix = %q", got)
	}
}
