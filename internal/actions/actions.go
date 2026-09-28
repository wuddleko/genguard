package actions

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wuddleko/genguard/internal/pathx"
)

type Env struct {
	Actions     bool
	Annotations bool
	Workspace   string
}

func Read() Env {
	return Env{
		Actions:     os.Getenv("GITHUB_ACTIONS") == "true",
		Annotations: os.Getenv("GENGUARD_ANNOTATIONS") != "false",
		Workspace:   strings.TrimSpace(os.Getenv("GITHUB_WORKSPACE")),
	}
}

func (e Env) Annotating() bool {
	return e.Actions && e.Annotations
}

func OpenGroup(w io.Writer, title string) {
	fmt.Fprintf(w, "::group::%s\n", groupTitle(title))
}

func CloseGroup(w io.Writer) {
	fmt.Fprintf(w, "::endgroup::\n")
}

func groupTitle(text string) string {
	// ##[group] decodes %0A, so a raw break is folded before EscapeData.
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, text)
	return EscapeData(text)
}

type Bracket struct {
	W     io.Writer
	Token string
}

func (b Bracket) Open() (int, error) {
	return fmt.Fprintf(b.W, "::stop-commands::%s\n", b.Token)
}

func (b Bracket) Close() (int, error) {
	return fmt.Fprintf(b.W, "::%s::\n", b.Token)
}

func Token() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func Annotation(file, title, message string) string {
	var b strings.Builder
	b.WriteString("::error")
	if file != "" || title != "" {
		b.WriteString(" ")
		sep := ""
		if file != "" {
			b.WriteString("file=")
			b.WriteString(EscapeProperty(file))
			sep = ","
		}
		if title != "" {
			b.WriteString(sep)
			b.WriteString("title=")
			b.WriteString(EscapeProperty(title))
		}
	}
	b.WriteString("::")
	b.WriteString(EscapeData(message))
	b.WriteString("\n")
	return b.String()
}

func WorkspaceFile(workspace, repoRoot, file string) string {
	file = strings.ReplaceAll(file, "\\", "/")
	if file == "" || repoRoot == "" || filepath.IsAbs(file) {
		return file
	}
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return file
	}
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return file
	}
	wsAbs, err := filepath.Abs(workspace)
	if err != nil {
		return file
	}
	prefix, ok := pathx.RelInside(wsAbs, rootAbs)
	if !ok {
		return file
	}
	if prefix == "." {
		return file
	}
	return filepath.ToSlash(filepath.Join(prefix, filepath.FromSlash(file)))
}

func EscapeData(s string) string {
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	).Replace(s)
}

func EscapeProperty(s string) string {
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
		":", "%3A",
		",", "%2C",
	).Replace(s)
}
