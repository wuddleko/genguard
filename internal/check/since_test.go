package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSinceDashRefIsNotAGitOption(t *testing.T) {
	root := gitRepo(t)
	for _, since := range []string{"--all", "-n"} {
		t.Run(since, func(t *testing.T) {
			logPath := filepath.Join(t.TempDir(), "git-args")
			installGitShim(t, "record")
			t.Setenv("GENGUARD_GIT_ARGS", logPath)

			_, err := mergeBase(commandLog{}, root, since)
			if err == nil || !strings.Contains(err.Error(), "bad --since ref") || gitFlagError(err.Error()) {
				t.Fatalf("mergeBase: %v", err)
			}
			assertRefNotGitOption(t, recordedGitArgs(t, logPath), since)
		})
	}
}

func TestMergeBasePassesResolvedSHA(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "f.txt", "ok\n")
	logPath := filepath.Join(t.TempDir(), "git-args")
	installGitShim(t, "record")
	t.Setenv("GENGUARD_GIT_ARGS", logPath)

	base, err := mergeBase(commandLog{}, root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	assertMergeBaseSawSHA(t, recordedGitArgs(t, logPath), "HEAD", base)
}

func gitFlagError(msg string) bool {
	return strings.Contains(msg, "unknown option") || strings.Contains(msg, "unrecognized argument")
}

func recordedGitArgs(t *testing.T, path string) [][]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSuffix(string(data), "\n")
	var invocations [][]string
	for _, block := range strings.Split(text, "\n\n") {
		block = strings.Trim(block, "\n")
		if block == "" {
			continue
		}
		invocations = append(invocations, strings.Split(block, "\n"))
	}
	if len(invocations) == 0 {
		t.Fatal("git shim recorded no arguments")
	}
	return invocations
}

func assertRefNotGitOption(t *testing.T, invocations [][]string, since string) {
	t.Helper()
	saw := false
	for _, args := range invocations {
		end := indexArg(args, "--end-of-options")
		for i, arg := range args {
			if arg == since && (end < 0 || i < end) {
				t.Fatalf("git saw %q before --end-of-options: %q", since, args)
			}
		}
		if containsArg(args, "--verify") {
			suffix := indexArg(args, since+"^{commit}")
			if end < 0 || suffix < end {
				t.Fatalf("rev-parse args = %q", args)
			}
			saw = true
		}
		if containsArg(args, "merge-base") {
			t.Fatalf("merge-base ran before the ref resolved: %q", args)
		}
	}
	if !saw {
		t.Fatal("rev-parse did not pass --end-of-options")
	}
}

func assertMergeBaseSawSHA(t *testing.T, invocations [][]string, since, sha string) {
	t.Helper()
	sawRev, sawMerge := false, false
	for _, args := range invocations {
		if containsArg(args, "--verify") {
			end := indexArg(args, "--end-of-options")
			if end < 0 || indexArg(args, since+"^{commit}") < end {
				t.Fatalf("rev-parse args = %q", args)
			}
			sawRev = true
		}
		if containsArg(args, "merge-base") {
			if !containsArg(args, "HEAD") || !containsArg(args, sha) || containsArg(args, since+"^{commit}") {
				t.Fatalf("merge-base args = %q, sha = %q", args, sha)
			}
			sawMerge = true
		}
	}
	if !sawRev || !sawMerge {
		t.Fatalf("invocations = %#v", invocations)
	}
}

func indexArg(args []string, want string) int {
	for i, arg := range args {
		if arg == want {
			return i
		}
	}
	return -1
}
