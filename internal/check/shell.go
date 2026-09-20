package check

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func shellInvocation(command string) (name string, args []string) {
	return shellInvocationFor(runtime.GOOS, exec.LookPath, os.Getenv("COMSPEC"), command)
}

func shellInvocationFor(
	goos string,
	lookPath func(string) (string, error),
	comspec string,
	command string,
) (name string, args []string) {
	if goos != "windows" {
		return "sh", []string{"-c", command}
	}
	for _, shell := range []string{"sh", "bash"} {
		path, err := lookPath(shell)
		if err == nil && path != "" {
			return path, []string{"-c", command}
		}
	}
	if strings.TrimSpace(comspec) == "" {
		comspec = "cmd.exe"
	}
	return comspec, []string{"/C", command}
}
