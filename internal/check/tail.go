package check

import "strings"

const (
	commandTailLines     = 50
	commandTailLineBytes = 8192
)

// tailRing keeps the last commandTailLines lines written to it.
// A line longer than commandTailLineBytes is cut there. The rest of that
// line is discarded until the next newline.
type tailRing struct {
	lines    []string
	cur      []byte
	drop     bool
	reported bool
	onLine   func(string)
}

func (r *tailRing) Write(p []byte) (int, error) {
	for _, b := range p {
		if r.drop {
			if b == '\n' {
				r.drop = false
			}
			continue
		}
		if b == '\n' {
			r.push(trimTrailingCR(r.cur))
			r.cur = r.cur[:0]
			continue
		}
		if len(r.cur) == commandTailLineBytes {
			r.push(string(r.cur))
			r.cur = r.cur[:0]
			r.drop = true
			continue
		}
		r.cur = append(r.cur, b)
		r.reported = false
	}
	return len(p), nil
}

// flush reports an unfinished line once. Later writes can report again. String does not.
func (r *tailRing) flush() {
	if r.reported || r.onLine == nil || r.drop || len(r.cur) == 0 {
		return
	}
	r.reported = true
	r.onLine(trimTrailingCR(r.cur))
}

func (r *tailRing) String() string {
	lines := r.lines
	hasPartial := !r.drop && len(r.cur) > 0
	if hasPartial && len(lines) == commandTailLines {
		lines = lines[1:]
	}
	if len(lines) == 0 && !hasPartial {
		return ""
	}
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	if hasPartial {
		if len(lines) > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(trimTrailingCR(r.cur))
	}
	b.WriteByte('\n')
	return b.String()
}

func (r *tailRing) push(line string) {
	if len(r.lines) == commandTailLines {
		copy(r.lines, r.lines[1:])
		r.lines[commandTailLines-1] = line
	} else {
		r.lines = append(r.lines, line)
	}
	if r.onLine != nil && !r.reported {
		r.onLine(line)
	}
	r.reported = false
}

func trimTrailingCR(b []byte) string {
	if len(b) > 0 && b[len(b)-1] == '\r' {
		b = b[:len(b)-1]
	}
	return string(b)
}
