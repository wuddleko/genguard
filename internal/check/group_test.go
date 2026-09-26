package check

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

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
