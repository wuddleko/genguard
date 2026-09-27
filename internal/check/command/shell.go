package command

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func shellInvocation(command string) (name string, args []string, cmdLine string) {
	return shellInvocationFor(runtime.GOOS, exec.LookPath, os.Getenv("COMSPEC"), command)
}

func shellInvocationFor(
	goos string,
	lookPath func(string) (string, error),
	comspec string,
	command string,
) (name string, args []string, cmdLine string) {
	if goos != "windows" {
		return "sh", []string{"-c", command}, ""
	}
	// sh on PATH, then bash.exe beside git.exe, then bash on PATH.
	// System32\bash.exe is the WSL stub and is never selected.
	if path, err := lookPath("sh"); err == nil && usableWindowsShell(path) {
		return path, []string{"-c", command}, ""
	}
	if path, err := lookPath("git"); err == nil {
		if bash := gitBashBeside(path); bash != "" {
			if found, err := lookPath(bash); err == nil && usableWindowsShell(found) {
				return found, []string{"-c", command}, ""
			}
		}
	}
	if path, err := lookPath("bash"); err == nil && usableWindowsShell(path) {
		return path, []string{"-c", command}, ""
	}
	if strings.TrimSpace(comspec) == "" {
		comspec = "cmd.exe"
	}
	return comspec, nil, windowsCmdLine(comspec, command)
}

// gitBashBeside is ../bin/bash.exe from git.exe, the Git for Windows layout
// where git.exe lives in cmd\ and bash.exe lives in bin\.
func gitBashBeside(gitExe string) string {
	slash := strings.ReplaceAll(gitExe, "/", `\`)
	dir, ok := parentDir(slash)
	if !ok {
		return ""
	}
	parent, ok := parentDir(dir)
	if !ok {
		return ""
	}
	return parent + `\bin\bash.exe`
}

func parentDir(path string) (string, bool) {
	i := strings.LastIndex(path, `\`)
	if i <= 0 {
		return "", false
	}
	return path[:i], true
}

func usableWindowsShell(path string) bool {
	return strings.TrimSpace(path) != "" && !system32Bash(path)
}

func system32Bash(path string) bool {
	path = strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	return strings.HasSuffix(path, `\windows\system32\bash.exe`)
}

func windowsCmdLine(comspec, command string) string {
	return quoteCmdExe(comspec) + " /C " + command
}

func quoteCmdExe(path string) string {
	if strings.ContainsAny(path, " \t\"") {
		return `"` + strings.ReplaceAll(path, `"`, `""`) + `"`
	}
	return path
}
