package check

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/wuddleko/genguard/internal/check/command"
)

type commandLog struct {
	w       io.Writer
	prefix  string
	timeout time.Duration
	// quiet keeps lines off the writer. A failure tail is written into the
	// group instead, and CommandTail is left empty.
	quiet bool
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
	// A raw break encoded as %0A is decoded again inside ##[group].
	// Fold it to a space, then escapeData so a literal %0A stays text.
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

// beginGroup opens a workflow group when Actions is logging this command.
// The returned function closes it. A nil writer leaves both as no-ops.
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

// commandStream copies finished command lines to a writer.
// On Actions the first line opens a stop-commands bracket. A token or write
// failure turns copying off so the caller keeps the tail.
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
	// A streamed log on Actions is bracketed with stop-commands. The token is
	// chosen before the child runs, so a rand failure skips the stream.
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
	// The label shares the first streamed line, inside the pause.
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

// finish closes the stop-commands bracket. The blank line follows the resume token.
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
	stream := newCommandStream(log, header)
	defer stream.finish()
	var onLine func(string)
	if stream.active {
		onLine = stream.onLine
	}
	tail, err := command.Run(root, commandText, onLine, timeout)
	if err != nil && stream.streamed() {
		tail = ""
	}
	if err != nil {
		return tail, newGenguardError("%s", err.Error())
	}
	return tail, nil
}
