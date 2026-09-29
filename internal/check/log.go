package check

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wuddleko/genguard/internal/actions"
	"github.com/wuddleko/genguard/internal/command"
)

type commandLog struct {
	w      io.Writer
	prefix string
	quiet  bool
	ctx    context.Context
	env    actions.Env
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
	if c.prefix == "" {
		return name
	}
	return c.prefix + name
}

func (c commandLog) groups() bool {
	return c.w != nil && c.env.Actions
}

func (c commandLog) beginGroup(name string) func() {
	if !c.groups() {
		return func() {}
	}
	actions.OpenGroup(c.w, c.groupTitle(name))
	return func() {
		actions.CloseGroup(c.w)
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
	actions.WriteBracketed(c.w, body.String())
	fmt.Fprint(c.w, "\n")
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

func newCommandStream(w io.Writer, header string, env actions.Env) *commandStream {
	s := &commandStream{w: w, header: header, active: w != nil}
	if s.active && env.Actions {
		s.token = actions.Token()
	}
	return s
}

func (s *commandStream) onLine(line string) {
	if s.stopFailed {
		return
	}
	if s.token != "" && !s.paused {
		n, werr := (actions.Bracket{W: s.w, Token: s.token}).Open()
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
		actions.Bracket{W: s.w, Token: s.token}.Close()
	}
	if s.headerWrote {
		io.WriteString(s.w, "\n")
	}
}

func (s *commandStream) streamed() bool {
	return s.w != nil && s.active
}

func runCommandContext(ctx context.Context, root, commandText string, log io.Writer, header string, timeout time.Duration, env actions.Env) (string, error) {
	stream := newCommandStream(log, header, env)
	defer stream.finish()
	var onLine func(string)
	if stream.active {
		onLine = stream.onLine
	}
	tail, err := command.Run(ctx, root, commandText, onLine, timeout)
	if err != nil && stream.streamed() {
		tail = ""
	}
	return tail, err
}
