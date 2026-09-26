package check

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCommandCopiesLines(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('line1'+chr(10)+'line2'+chr(10)); sys.exit(3)"`, &buf)
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

	tail, err := runCommand(root, command, nil)
	if err == nil || err.Error() != "command failed (exit 1)" {
		t.Fatalf("err = %v", err)
	}
	if tail != "::error file=evil.go::hijacked\n" {
		t.Fatalf("tail = %q", tail)
	}

	var buf bytes.Buffer
	tail, err = runCommand(root, command, &buf)
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

	tail, err := runCommand(root, command, nil)
	if err == nil || err.Error() != "command failed (exit 1)" {
		t.Fatalf("err = %v", err)
	}
	if tail != want {
		t.Fatalf("tail = %q", tail)
	}

	var buf bytes.Buffer
	tail, err = runCommand(root, command, &buf)
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
	tail, err := runCommand(root, command, &buf)
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
	tail, err := runCommand(root, "exit 4", &buf)
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
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('hello'); sys.exit(3)"`, &buf)
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
	tail, err := runCommand(root, `python3 -c "import sys; sys.stderr.write('hello'+chr(10))"`, &buf)
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
	tail, err := runCommand(root, "exit 4", &buf)
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
	tail, err := runCommand(filepath.Join(root, "missing"), "true", &buf)
	if err == nil || err.Error() != "command failed (exit 1): no output" {
		t.Fatalf("err = %v", err)
	}
	if tail != "" || buf.Len() != 0 {
		t.Fatalf("tail = %q log = %q", tail, buf.String())
	}
}
