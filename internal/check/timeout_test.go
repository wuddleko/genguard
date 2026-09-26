package check

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuddleko/genguard/internal/config"
)

func TestCheckTimeoutRunsLaterGroup(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "left.txt", "ok\n")
	writeTracked(t, root, "right.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{
			{
				Name:    "greeting",
				Command: `python3 -c "import sys,time; sys.stderr.write('line1'+chr(10)); sys.stderr.flush(); time.sleep(5)"`,
				Outputs: []string{"left.txt"},
			},
			{
				Name:    "other",
				Command: "true",
				Outputs: []string{"right.txt"},
			},
		},
	}

	start := time.Now()
	result, err := checkConfig(cfg, "", nil, commandLog{timeout: 200 * time.Millisecond})
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	if len(result.Groups) != 2 {
		t.Fatalf("groups = %+v", result.Groups)
	}
	greeting := result.Groups[0]
	if greeting.Status != GroupError || greeting.Err == nil || greeting.Err.Error() != "command timed out after 200ms" {
		t.Fatalf("greeting = %+v", greeting)
	}
	if greeting.CommandTail != "line1\n" {
		t.Fatalf("tail = %q", greeting.CommandTail)
	}
	other := result.Groups[1]
	if other.Status != GroupOK || other.Err != nil || other.CommandTail != "" {
		t.Fatalf("other = %+v", other)
	}
}

func TestCheckTimeoutWriterDropsTail(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "left.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "import sys,time; sys.stderr.write('line1'+chr(10)); sys.stderr.flush(); time.sleep(5)"`,
			Outputs: []string{"left.txt"},
		}},
	}
	var buf bytes.Buffer
	start := time.Now()
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf, timeout: 200 * time.Millisecond})
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err != nil {
		t.Fatal(err)
	}
	greeting := result.Groups[0]
	if greeting.Status != GroupError || greeting.Err == nil || greeting.Err.Error() != "command timed out after 200ms" {
		t.Fatalf("greeting = %+v", greeting)
	}
	if greeting.CommandTail != "" {
		t.Fatalf("tail = %q", greeting.CommandTail)
	}
	if buf.String() != "greeting:\nline1\n\n" {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestCheckTimeoutSkipDoesNotRun(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "queries/q.sql", "select 1;\n")
	writeTracked(t, root, "out.txt", "ok\n")
	writeTracked(t, root, "genguard.yaml", "groups: []\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "quiet",
			Command: `python3 -c "import sys,time; sys.stderr.write('ran'+chr(10)); sys.stderr.flush(); time.sleep(5)"`,
			Inputs:  []string{"queries/"},
			Outputs: []string{"out.txt"},
		}},
	}

	start := time.Now()
	result, err := checkConfig(cfg, "HEAD", nil, commandLog{timeout: 200 * time.Millisecond})
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0; groups = %+v", result.ExitCode(), result.Groups)
	}
	group := result.Groups[0]
	if group.Status != GroupSkipped || group.Err != nil || group.CommandTail != "" {
		t.Fatalf("group = %+v", group)
	}
}

func TestCheckTimeoutCleanLeavesWipe(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "generated/hello.txt", "hello\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "import time; time.sleep(5)"`,
			Outputs: []string{"generated/hello.txt"},
			Clean:   true,
		}},
	}

	start := time.Now()
	result, err := checkConfig(cfg, "", nil, commandLog{timeout: 200 * time.Millisecond})
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	const want = "command failed after cleaning outputs: command timed out after 200ms"
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != want {
		t.Fatalf("group = %+v", group)
	}
	if _, statErr := os.Stat(filepath.Join(root, "generated", "hello.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("wiped file stat = %v", statErr)
	}
}

func TestCheckTimeoutStillDiffs(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "generated/hello.txt", "old\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "f=open('generated/hello.txt','wb'); f.write(b'new\n'); f.flush(); f.close(); import time; time.sleep(5)"`,
			Outputs: []string{"generated/hello.txt"},
		}},
	}

	start := time.Now()
	result, err := checkConfig(cfg, "", nil, commandLog{timeout: 200 * time.Millisecond})
	if time.Since(start) >= time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "command timed out after 200ms" {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Drifts) != 1 || group.Drifts[0].Kind != "modified" || group.Drifts[0].Path != "generated/hello.txt" {
		t.Fatalf("drifts = %+v", group.Drifts)
	}
	data, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("file = %q", data)
	}
}
