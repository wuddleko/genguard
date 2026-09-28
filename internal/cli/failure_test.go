package cli

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/actions"
	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestFinishConfigJSONWithoutRepo(t *testing.T) {
	var out, errBuf bytes.Buffer
	path := filepath.Join(t.TempDir(), "genguard.yaml")
	code := finishConfig(&out, &errBuf, check.ConfigResult{}, path, t.TempDir(), "ok", true, actions.Env{})
	if code != 2 || out.Len() != 0 || !strings.Contains(errBuf.String(), "not a git work tree") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
}

func TestFinishConfigAnnotationWithoutRepo(t *testing.T) {
	var out, errBuf bytes.Buffer
	path := filepath.Join(t.TempDir(), "genguard.yaml")
	env := actions.Env{Actions: true, Annotations: true}
	code := finishConfig(&out, &errBuf, check.ConfigResult{}, path, t.TempDir(), "Generated files match the generators.", false, env)
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Generated files match the generators.") {
		t.Fatalf("stdout = %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "not a git work tree") || !strings.Contains(errBuf.String(), "::error") {
		t.Fatalf("stderr = %q", errBuf.String())
	}
}

func TestStopCommandsAddsTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	withoutWorkflowCommands(&buf, actions.Env{Actions: true}, func(w io.Writer) {
		fmt.Fprint(w, "::not-a-command")
	})
	text := buf.String()
	if !strings.Contains(text, "::stop-commands::") || !strings.Contains(text, "::not-a-command\n::") {
		t.Fatalf("text = %q", text)
	}
	if !strings.HasSuffix(text, "::\n") {
		t.Fatalf("text = %q", text)
	}
}

func TestFinishConfigJSONRelativePathWithoutCwd(t *testing.T) {
	testutil.WithoutWorkingDirectory(t)
	var out, errBuf bytes.Buffer
	code := finishConfig(&out, &errBuf, check.ConfigResult{}, "genguard.yaml", t.TempDir(), "ok", true, actions.Env{})
	if code != 2 || out.Len() != 0 || !strings.Contains(errBuf.String(), "error:") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), errBuf.String())
	}
}
