package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/actions"
)

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
