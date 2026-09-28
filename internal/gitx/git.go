package gitx

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/wuddleko/genguard/internal/check/command"
)

// Run runs git in root. A canceled command returns command.ErrInterrupted.
func Run(ctx context.Context, root string, args ...string) (string, int, error) {
	stdout, stderr, err := command.Output(ctx, "git", append([]string{"-C", root}, args...)...)
	if errors.Is(err, command.ErrInterrupted) {
		return "", 0, err
	}
	return Result(stdout, stderr, err)
}

func Result(stdout, stderr string, err error) (string, int, error) {
	if err == nil {
		return stdout, 0, nil
	}
	var exitErr *exec.ExitError
	if errorsAsExit(err, &exitErr) {
		code := exitErr.ExitCode()
		// Exit 1 with a patch is a diff. A CRLF warning on stderr must not join it.
		if code == 1 {
			if strings.TrimSpace(stdout) == "" && strings.TrimSpace(stderr) != "" {
				return stderr, code, nil
			}
			return stdout, code, nil
		}
		if strings.TrimSpace(stderr) != "" {
			return stderr, code, nil
		}
		return stdout, code, nil
	}
	if strings.TrimSpace(stderr) != "" {
		return stderr, -1, err
	}
	return stdout, -1, err
}

func ParseNameList(out string) []string {
	if out == "" {
		return nil
	}
	parts := strings.Split(out, "\x00")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			names = append(names, part)
		}
	}
	return names
}

func Detail(out, fallback string) string {
	detail := strings.TrimSpace(out)
	if detail == "" {
		return fallback
	}
	return detail
}

func errorsAsExit(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}
