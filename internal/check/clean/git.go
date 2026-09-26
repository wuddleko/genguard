package clean

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func gitNames(root string, args ...string) ([]string, error) {
	out, code, err := git(root, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", gitDetail(out, "git failed"))
	}
	return parseGitNameList(out), nil
}

func parseGitNameList(out string) []string {
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

func gitDetail(out, fallback string) string {
	detail := strings.TrimSpace(out)
	if detail == "" {
		return fallback
	}
	return detail
}

func git(root string, args ...string) (string, int, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return gitResult(stdout.String(), stderr.String(), err)
}

func gitResult(stdout, stderr string, err error) (string, int, error) {
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

func errorsAsExit(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		return false
	}
	*target = exitErr
	return true
}
