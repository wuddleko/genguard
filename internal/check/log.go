package check

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/wuddleko/genguard/internal/check/command"
)

type commandLog struct {
	w         io.Writer
	prefix    string
	timeout   time.Duration
	quiet     bool
	ctx       context.Context
	toolCache *toolCache
}

func (c commandLog) withToolCache() commandLog {
	if c.toolCache == nil {
		c.toolCache = &toolCache{}
	}
	return c
}

func (c commandLog) canceled() bool {
	return c.ctx != nil && c.ctx.Err() != nil
}

func (c commandLog) label(name string) string {
	text := name + ":"
	if c.prefix != "" {
		text = c.prefix + text
	}
	return text
}

func (c commandLog) groupTitle(name string) string {
	text := name
	if c.prefix != "" {
		text = c.prefix + name
	}
	// ##[group] decodes %0A, so a raw break is folded before escapeData.
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, text)
	return escapeData(text)
}

func (c commandLog) groups() bool {
	return c.w != nil && os.Getenv("GITHUB_ACTIONS") == "true"
}

func (c commandLog) beginGroup(name string) func() {
	if !c.groups() {
		return func() {}
	}
	fmt.Fprintf(c.w, "::group::%s\n", c.groupTitle(name))
	return func() {
		fmt.Fprintf(c.w, "::endgroup::\n")
	}
}

func (c commandLog) writeGroupedTail(name, tail string) {
	var body strings.Builder
	body.WriteString(c.label(name))
	body.WriteByte('\n')
	body.WriteString(tail)
	if !strings.HasSuffix(tail, "\n") {
		body.WriteByte('\n')
	}
	text := body.String()
	token, err := workflowStopToken()
	if err != nil {
		fmt.Fprint(c.w, text)
		fmt.Fprint(c.w, "\n")
		return
	}
	fmt.Fprintf(c.w, "::stop-commands::%s\n", token)
	fmt.Fprint(c.w, text)
	fmt.Fprintf(c.w, "::%s::\n", token)
	fmt.Fprint(c.w, "\n")
}

func workflowStopToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

type commandStream struct {
	w           io.Writer
	header      string
	token       string
	paused      bool
	headerWrote bool
	stopFailed  bool
	active      bool
}

func newCommandStream(w io.Writer, header string) *commandStream {
	s := &commandStream{w: w, header: header, active: w != nil}
	if s.active && os.Getenv("GITHUB_ACTIONS") == "true" {
		token, err := workflowStopToken()
		if err != nil {
			s.active = false
			return s
		}
		s.token = token
	}
	return s
}

func (s *commandStream) onLine(line string) {
	if s.stopFailed {
		return
	}
	if s.token != "" && !s.paused {
		n, werr := fmt.Fprintf(s.w, "::stop-commands::%s\n", s.token)
		if n > 0 {
			s.paused = true
		}
		if werr != nil {
			s.stopFailed = true
			s.active = false
			return
		}
	}
	if s.header != "" && !s.headerWrote {
		if _, werr := fmt.Fprintln(s.w, s.header); werr != nil {
			s.stopFailed = true
			s.active = false
			return
		}
		s.headerWrote = true
	}
	fmt.Fprintln(s.w, line)
}

func (s *commandStream) finish() {
	if s.paused {
		fmt.Fprintf(s.w, "::%s::\n", s.token)
	}
	if s.headerWrote {
		io.WriteString(s.w, "\n")
	}
}

func (s *commandStream) streamed() bool {
	return s.w != nil && s.active
}

func RunCommand(root, commandText string) (string, error) {
	return runCommand(root, commandText, nil, "", 0)
}

func runCommand(root, commandText string, log io.Writer, header string, timeout time.Duration) (string, error) {
	return runCommandContext(nil, root, commandText, log, header, timeout)
}

func runCommandContext(ctx context.Context, root, commandText string, log io.Writer, header string, timeout time.Duration) (string, error) {
	stream := newCommandStream(log, header)
	defer stream.finish()
	var onLine func(string)
	if stream.active {
		onLine = stream.onLine
	}
	tail, err := command.RunContext(ctx, root, commandText, onLine, timeout)
	if err != nil && stream.streamed() {
		tail = ""
	}
	if errors.Is(err, command.ErrInterrupted) {
		return tail, errInterrupted
	}
	if err != nil {
		return tail, newGenguardError("%s", err.Error())
	}
	return tail, nil
}
