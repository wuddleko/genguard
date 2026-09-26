package check

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRunCommandCopiesLines(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('line1'+chr(10)+'line2'+chr(10)); sys.exit(3)"`, &buf, "", 0)
	var genguardErr *GenguardError
	if !errors.As(err, &genguardErr) {
		t.Fatalf("err = %v", err)
	}
	if err.Error() != "command failed (exit 3)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if buf.String() != "line1\nline2\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRunCommandCopiesWorkflowLines(t *testing.T) {
	root := t.TempDir()
	command := `python3 -c "import sys; sys.stderr.write('::error file=evil.go::hijacked'+chr(10)); sys.exit(1)"`

	tail, err := runCommand(root, command, nil, "", 0)
	if err == nil || err.Error() != "command failed (exit 1)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "::error file=evil.go::hijacked\n" {
		t.Fatalf("tail = %q", tail)
	}

	var buf bytes.Buffer
	tail, err = runCommand(root, command, &buf, "", 0)
	if err == nil || err.Error() != "command failed (exit 1)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if buf.String() != "::error file=evil.go::hijacked\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRunCommandCopiesWorkflowLinesWithCR(t *testing.T) {
	root := t.TempDir()
	command := `python3 -c "import sys; sys.stderr.write('note'+chr(13)+'::error file=evil.go::hijacked'+chr(13)+'::stop-commands::hijack'+chr(10)); sys.exit(1)"`
	const want = "note\r::error file=evil.go::hijacked\r::stop-commands::hijack\n"

	tail, err := runCommand(root, command, nil, "", 0)
	if err == nil || err.Error() != "command failed (exit 1)" {
		t.Fatalf("err = %v", err)
	}
	if tail != want {
		t.Fatalf("tail = %q", tail)
	}

	var buf bytes.Buffer
	tail, err = runCommand(root, command, &buf, "", 0)
	if err == nil || err.Error() != "command failed (exit 1)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if buf.String() != want {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRunCommandPausesWorkflowCommands(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := t.TempDir()
	var buf bytes.Buffer
	command := `python3 -c "import sys; sys.stderr.write('::error file=evil.go::hijacked'+chr(10)+'note'+chr(13)+'::stop-commands::hijack'+chr(10)); sys.exit(1)"`
	tail, err := runCommand(root, command, &buf, "", 0)
	if err == nil || err.Error() != "command failed (exit 1)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	_, body := splitPausedCommandLog(t, buf.String())
	if body != "::error file=evil.go::hijacked\nnote\r::stop-commands::hijack\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestRunCommandPauseSkipsQuietCommand(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, "exit 4", &buf, "", 0)
	if err == nil || err.Error() != "command failed (exit 4): no output" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" || buf.Len() != 0 {
		t.Fatalf("tail = %q log = %q", tail, buf.String())
	}
}

func splitPausedCommandLog(t *testing.T, text string) (token, body string) {
	t.Helper()
	const prefix = "::stop-commands::"
	first, rest, ok := strings.Cut(text, "\n")
	if !ok || !strings.HasPrefix(first, prefix) {
		t.Fatalf("log = %q", text)
	}
	token = strings.TrimPrefix(first, prefix)
	if len(token) != 32 || strings.Trim(token, "0123456789abcdef") != "" {
		t.Fatalf("token = %q", token)
	}
	body, ok = strings.CutSuffix(rest, "::"+token+"::\n")
	if !ok {
		t.Fatalf("log = %q", text)
	}
	return token, body
}

func TestRunCommandFlushesPartialLine(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('hello'); sys.exit(3)"`, &buf, "", 0)
	if err == nil || err.Error() != "command failed (exit 3)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if buf.String() != "hello\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRunCommandSuccessCopiesLines(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('hello'+chr(10))"`, &buf, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if buf.String() != "hello\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRunCommandWriterWithNoOutput(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, "exit 4", &buf, "", 0)
	if err == nil || err.Error() != "command failed (exit 4): no output" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" || buf.Len() != 0 {
		t.Fatalf("tail = %q log = %q", tail, buf.String())
	}
}

func TestRunCommandWriterStartFailure(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(filepath.Join(root, "missing"), "true", &buf, "", 0)
	if err == nil || err.Error() != "command failed (exit 1): no output" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" || buf.Len() != 0 {
		t.Fatalf("tail = %q log = %q", tail, buf.String())
	}
}

func TestRunCommandWritesHeaderBeforeLines(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('hello'+chr(10)); sys.exit(3)"`, &buf, "greeting:", 0)
	if err == nil || err.Error() != "command failed (exit 3)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if buf.String() != "greeting:\nhello\n\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRunCommandHeaderSkipsQuietCommand(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, "exit 4", &buf, "greeting:", 0)
	if err == nil || err.Error() != "command failed (exit 4): no output" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" || buf.Len() != 0 {
		t.Fatalf("tail = %q log = %q", tail, buf.String())
	}
}

func TestRunCommandHeaderFollowsStopCommands(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := t.TempDir()
	var buf bytes.Buffer
	header := "::error file=evil.go::hijacked\n::stop-commands::fixed"
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('hello'+chr(10)); sys.exit(1)"`, &buf, header, 0)
	if err == nil || err.Error() != "command failed (exit 1)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	text := buf.String()
	first, rest, ok := strings.Cut(text, "\n")
	if !ok || !strings.HasPrefix(first, "::stop-commands::") {
		t.Fatalf("log = %q", text)
	}
	token := strings.TrimPrefix(first, "::stop-commands::")
	body, after, ok := strings.Cut(rest, "::"+token+"::\n")
	if !ok {
		t.Fatalf("log = %q", text)
	}
	if body != header+"\nhello\n" {
		t.Fatalf("body = %q", body)
	}
	if after != "\n" {
		t.Fatalf("after = %q", after)
	}
}

func TestRunCommandTimeoutAllowsFastSuccess(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('hello'+chr(10))"`, &buf, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if tail != "" {
		t.Fatalf("tail = %q", tail)
	}
	if buf.String() != "hello\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestRunCommandTimeoutAllowsFastFailure(t *testing.T) {
	root := t.TempDir()
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('line1'+chr(10)); sys.exit(3)"`, nil, "", time.Second)
	if err == nil || err.Error() != "command failed (exit 3)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "line1\n" {
		t.Fatalf("tail = %q", tail)
	}
}

func TestRunCommandTimeoutStartFailure(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(filepath.Join(root, "missing"), "true", &buf, "", time.Second)
	if err == nil || err.Error() != "command failed (exit 1): no output" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" || buf.Len() != 0 {
		t.Fatalf("tail = %q log = %q", tail, buf.String())
	}
}

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

func pythonCommand(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return "python3 " + quoteForShell(path)
}

func quoteForShell(arg string) string {
	_, args := shellInvocation("")
	return quoteForInvocation(args, arg)
}

func quoteForInvocation(args []string, arg string) string {
	if len(args) > 0 && args[0] == "/C" {
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
