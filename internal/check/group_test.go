package check

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuddleko/genguard/internal/config"
)

func TestWorkflowGroupWrapsEachRun(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "queries/q.sql", "select 1;\n")
	writeTracked(t, root, "left.txt", "ok\n")
	writeTracked(t, root, "mid.txt", "ok\n")
	writeTracked(t, root, "right.txt", "ok\n")
	writeTracked(t, root, "genguard.yaml", "groups: []\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{
			{Name: "greeting", Command: "true", Outputs: []string{"left.txt"}},
			{
				Name:    "quiet",
				Command: `python3 -c "open('ran','w').close()"`,
				Inputs:  []string{"queries/"},
				Outputs: []string{"mid.txt"},
			},
			{Name: "other", Command: "true", Outputs: []string{"right.txt"}},
		},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "HEAD", nil, commandLog{w: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups[0].Status != GroupOK || result.Groups[1].Status != GroupSkipped || result.Groups[2].Status != GroupOK {
		t.Fatalf("groups = %+v", result.Groups)
	}
	const want = "::group::greeting\n::endgroup::\n::group::other\n::endgroup::\n"
	if buf.String() != want {
		t.Fatalf("log = %q", buf.String())
	}
	if _, statErr := os.Stat(filepath.Join(root, "ran")); !os.IsNotExist(statErr) {
		t.Fatal("skipped group ran")
	}
}

func TestWorkflowGroupUsesCallerPath(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: "true",
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	_, err := checkConfig(cfg, "", nil, commandLog{w: &buf, prefix: "services/%0A/genguard.yaml: "})
	if err != nil {
		t.Fatal(err)
	}
	const want = "::group::services/%250A/genguard.yaml: greeting\n::endgroup::\n"
	if buf.String() != want {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestWorkflowGroupTitleIsOneLine(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "::error file=evil.go::hijacked\n%0A##[endgroup]%0D##[error]",
			Command: "true",
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	_, err := checkConfig(cfg, "", nil, commandLog{w: &buf})
	if err != nil {
		t.Fatal(err)
	}
	const want = "::group::::error file=evil.go::hijacked %250A##[endgroup]%250D##[error]\n::endgroup::\n"
	if buf.String() != want {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestWorkflowGroupClosesAfterCleanFailure(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "right.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{
			{
				Name:    "greeting",
				Command: `python3 -c "open('ran','w').close()"`,
				Outputs: []string{"../outside"},
				Clean:   true,
			},
			{Name: "other", Command: "true", Outputs: []string{"right.txt"}},
		},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf})
	if err != nil {
		t.Fatal(err)
	}
	greeting := result.Groups[0]
	if greeting.Status != GroupError || greeting.Err == nil || !bytes.Contains([]byte(greeting.Err.Error()), []byte("clean refuses")) {
		t.Fatalf("greeting = %+v", greeting)
	}
	if result.Groups[1].Status != GroupOK {
		t.Fatalf("other = %+v", result.Groups[1])
	}
	const want = "::group::greeting\n::endgroup::\n::group::other\n::endgroup::\n"
	if buf.String() != want {
		t.Fatalf("log = %q", buf.String())
	}
	if _, statErr := os.Stat(filepath.Join(root, "ran")); !os.IsNotExist(statErr) {
		t.Fatal("command ran after clean failed")
	}
}

func TestWorkflowGroupStaysQuietWithoutActions(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: "true",
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	if _, err := checkConfig(cfg, "", nil, commandLog{w: &buf}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Fatalf("log = %q", buf.String())
	}

	t.Setenv("GITHUB_ACTIONS", "true")
	result, err := checkConfig(cfg, "", nil, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups[0].Status != GroupOK {
		t.Fatalf("group = %+v", result.Groups[0])
	}
}

func TestWorkflowGroupQuietFailureKeepsTailInside(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "import sys; sys.stderr.write('line1'+chr(10)); sys.exit(3)"`,
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf, quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "command failed (exit 3)" {
		t.Fatalf("group = %+v", group)
	}
	if group.CommandTail != "" {
		t.Fatalf("tail = %q", group.CommandTail)
	}
	body, after := splitWorkflowGroup(t, buf.String(), "greeting")
	if after != "" {
		t.Fatalf("after = %q", after)
	}
	_, paused := splitPausedCommandLog(t, strings.TrimSuffix(body, "\n"))
	if paused != "greeting:\nline1\n" {
		t.Fatalf("paused = %q", paused)
	}
	if strings.Count(buf.String(), "line1") != 1 {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestWorkflowGroupQuietErrorStaysInsidePause(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "import sys; sys.stderr.write('::error file=evil.go::hijacked'+chr(10)); sys.exit(1)"`,
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf, prefix: "services/api/genguard.yaml: ", quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups[0].CommandTail != "" {
		t.Fatalf("tail = %q", result.Groups[0].CommandTail)
	}
	body, after := splitWorkflowGroup(t, buf.String(), "services/api/genguard.yaml: greeting")
	if after != "" {
		t.Fatalf("after = %q", after)
	}
	_, paused := splitPausedCommandLog(t, strings.TrimSuffix(body, "\n"))
	if paused != "services/api/genguard.yaml: greeting:\n::error file=evil.go::hijacked\n" {
		t.Fatalf("paused = %q", paused)
	}
}

func TestWorkflowGroupVerboseFailureStreamsOnce(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "import sys; sys.stderr.write('line1'+chr(10)); sys.exit(3)"`,
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "command failed (exit 3)" {
		t.Fatalf("group = %+v", group)
	}
	if group.CommandTail != "" {
		t.Fatalf("tail = %q", group.CommandTail)
	}
	body, after := splitWorkflowGroup(t, buf.String(), "greeting")
	if after != "" {
		t.Fatalf("after = %q", after)
	}
	_, paused := splitPausedCommandLog(t, strings.TrimSuffix(body, "\n"))
	if paused != "greeting:\nline1\n" {
		t.Fatalf("paused = %q", paused)
	}
	if strings.Count(buf.String(), "line1") != 1 {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestWorkflowGroupQuietSuccessIsEmpty(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "import sys; sys.stderr.write('line1'+chr(10))"`,
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf, quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups[0].Status != GroupOK || result.Groups[0].CommandTail != "" {
		t.Fatalf("group = %+v", result.Groups[0])
	}
	if buf.String() != "::group::greeting\n::endgroup::\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestWorkflowGroupQuietNoOutputIsEmpty(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: "exit 4",
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf, quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "command failed (exit 4): no output" {
		t.Fatalf("group = %+v", group)
	}
	if group.CommandTail != "" {
		t.Fatalf("tail = %q", group.CommandTail)
	}
	if buf.String() != "::group::greeting\n::endgroup::\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestWorkflowGroupQuietTimeoutKeepsTailInside(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "import sys,time; sys.stderr.write('line1'+chr(10)); sys.stderr.flush(); time.sleep(5)"`,
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	start := time.Now()
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf, quiet: true, timeout: 200 * time.Millisecond})
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "command timed out after 200ms" {
		t.Fatalf("group = %+v", group)
	}
	if group.CommandTail != "" {
		t.Fatalf("tail = %q", group.CommandTail)
	}
	body, after := splitWorkflowGroup(t, buf.String(), "greeting")
	if after != "" {
		t.Fatalf("after = %q", after)
	}
	_, paused := splitPausedCommandLog(t, strings.TrimSuffix(body, "\n"))
	if paused != "greeting:\nline1\n" {
		t.Fatalf("paused = %q", paused)
	}
}

func TestWorkflowGroupQuietTailStaysWithoutActions(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "import sys; sys.stderr.write('line1'+chr(10)); sys.exit(3)"`,
			Outputs: []string{"out.txt"},
		}},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf, quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "command failed (exit 3)" {
		t.Fatalf("group = %+v", group)
	}
	if group.CommandTail != "line1\n" {
		t.Fatalf("tail = %q", group.CommandTail)
	}
	if buf.Len() != 0 {
		t.Fatalf("log = %q", buf.String())
	}
}

func splitWorkflowGroup(t *testing.T, text, title string) (body, after string) {
	t.Helper()
	open := "::group::" + title + "\n"
	if !strings.HasPrefix(text, open) {
		t.Fatalf("log = %q", text)
	}
	var ok bool
	body, after, ok = strings.Cut(text[len(open):], "::endgroup::\n")
	if !ok {
		t.Fatalf("log = %q", text)
	}
	return body, after
}
