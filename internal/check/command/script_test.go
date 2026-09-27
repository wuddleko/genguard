package command

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func timeoutScript(pidPath string) string {
	return "import os, subprocess, sys\n" +
		"sys.stderr.write('line1\\n')\n" +
		"sys.stderr.flush()\n" +
		"child = subprocess.Popen(\n" +
		"    [sys.executable, '-c', 'import time; time.sleep(15)'],\n" +
		"    stdin=sys.stdin, stdout=sys.stdout, stderr=sys.stderr, close_fds=False)\n" +
		"fd = os.open(" + strconv.Quote(pidPath) + ", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)\n" +
		"os.write(fd, str(child.pid).encode())\n" +
		"os.close(fd)\n" +
		"os._exit(0)\n"
}

func holdStdoutScript(pidPath string) string {
	return "import os, subprocess, sys\n" +
		"sys.stdout.write('done\\n')\n" +
		"sys.stdout.flush()\n" +
		"child = subprocess.Popen(\n" +
		"    [sys.executable, '-c', 'import sys,time; time.sleep(5); sys.stdout.write(\"later\"+chr(10)); sys.stdout.flush()'],\n" +
		"    stdin=subprocess.DEVNULL, stdout=sys.stdout, stderr=subprocess.DEVNULL, close_fds=False)\n" +
		"fd = os.open(" + strconv.Quote(pidPath) + ", os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)\n" +
		"os.write(fd, str(child.pid).encode())\n" +
		"os.close(fd)\n" +
		"os._exit(0)\n"
}

func pythonCommand(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return "python3 " + quoteForShell(path)
}

func quoteForShell(arg string) string {
	_, args, cmdLine := shellInvocation("")
	return quoteForInvocation(args, cmdLine, arg)
}

func quoteForInvocation(args []string, cmdLine, arg string) string {
	if cmdLine != "" || (len(args) > 0 && args[0] == "/C") {
		return cmdQuote(arg)
	}
	return shSingle(arg)
}

func shSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func cmdQuote(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	s = strings.ReplaceAll(s, `"`, `""`)
	return `"` + s + `"`
}
