package check

import (
	"errors"
	"testing"
)

func TestShellInvocationUnix(t *testing.T) {
	t.Parallel()
	name, args := shellInvocationFor("linux", nil, "", "buf generate")
	if name != "sh" {
		t.Fatalf("name = %q, want sh", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "buf generate" {
		t.Fatalf("args = %q", args)
	}
}

func TestShellInvocationWindowsPrefersSh(t *testing.T) {
	t.Parallel()
	look := func(n string) (string, error) {
		if n == "sh" {
			return `C:\Git\bin\sh.exe`, nil
		}
		return "", errors.New("not found")
	}
	name, args := shellInvocationFor("windows", look, `C:\Windows\system32\cmd.exe`, "buf generate")
	if name != `C:\Git\bin\sh.exe` {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "buf generate" {
		t.Fatalf("args = %q", args)
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
	name, args := shellInvocationFor("windows", look, `C:\Windows\system32\cmd.exe`, "make generate")
	if name != `C:\Git\bin\bash.exe` {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 2 || args[0] != "-c" || args[1] != "make generate" {
		t.Fatalf("args = %q", args)
	}
}

func TestShellInvocationWindowsFallsBackToComspec(t *testing.T) {
	t.Parallel()
	look := func(string) (string, error) {
		return "", errors.New("not found")
	}
	name, args := shellInvocationFor("windows", look, `D:\cmd.exe`, "buf generate")
	if name != `D:\cmd.exe` {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 2 || args[0] != "/C" || args[1] != "buf generate" {
		t.Fatalf("args = %q", args)
	}
}

func TestShellInvocationWindowsDefaultCmd(t *testing.T) {
	t.Parallel()
	look := func(string) (string, error) {
		return "", errors.New("not found")
	}
	name, args := shellInvocationFor("windows", look, "  ", "true")
	if name != "cmd.exe" {
		t.Fatalf("name = %q", name)
	}
	if len(args) != 2 || args[0] != "/C" || args[1] != "true" {
		t.Fatalf("args = %q", args)
	}
}
