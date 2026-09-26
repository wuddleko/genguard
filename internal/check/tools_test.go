package check

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wuddleko/genguard/internal/config"
)

func TestToolMismatchSkipsCleanAndRunsNextGroup(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "gen/out.txt", "keep\n")
	writeTracked(t, root, "right.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Tools: []config.Tool{{
			Name:    "buf",
			Version: "1.32.0",
			Command: `python3 -c "print('1.28.1')"`,
		}},
		Groups: []config.Group{
			{
				Name:    "protobuf",
				Command: `python3 -c "open('ran','w').close(); open('gen/out.txt','w').write('gone\n')"`,
				Outputs: []string{"gen/"},
				Clean:   true,
				Tools:   []string{"buf"},
			},
			{
				Name:    "other",
				Command: `python3 -c "open('second','w').close()"`,
				Outputs: []string{"right.txt"},
			},
		},
	}

	result, err := checkConfig(cfg, "", nil, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", result.ExitCode())
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "buf: want 1.32.0, have 1.28.1" {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Tools) != 1 || group.Tools[0] != (ToolResult{Name: "buf", Want: "1.32.0", Have: "1.28.1"}) {
		t.Fatalf("tools = %+v", group.Tools)
	}
	if group.CommandTail != "" {
		t.Fatalf("tail = %q", group.CommandTail)
	}
	assertFile(t, filepath.Join(root, "gen", "out.txt"), "keep\n")
	assertAbsent(t, filepath.Join(root, "ran"))
	other := result.Groups[1]
	if other.Status != GroupOK {
		t.Fatalf("other = %+v", other)
	}
	if _, err := os.Stat(filepath.Join(root, "second")); err != nil {
		t.Fatal(err)
	}
}

func TestToolMatchCleansAndRuns(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "gen/out.txt", "old\n")
	writeTracked(t, root, "gen/extra.txt", "keep\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Tools: []config.Tool{{
			Name:    "buf",
			Version: "v1.2.3",
			Command: `python3 -c "print('release v1.2.3')"`,
		}},
		Groups: []config.Group{{
			Name:    "protobuf",
			Command: `python3 -c "open('gen/out.txt','w').write('new\n')"`,
			Outputs: []string{"gen/"},
			Clean:   true,
			Tools:   []string{"buf"},
		}},
	}

	result, err := checkConfig(cfg, "", nil, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupDrift {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Tools) != 1 || group.Tools[0] != (ToolResult{Name: "buf", Want: "1.2.3", Have: "1.2.3"}) {
		t.Fatalf("tools = %+v", group.Tools)
	}
	assertFile(t, filepath.Join(root, "gen", "out.txt"), "new\n")
	assertAbsent(t, filepath.Join(root, "gen", "extra.txt"))
}

func TestUnpinnedToolIsRecordedAndRuns(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "right.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Tools: []config.Tool{{
			Name:    "sqlc",
			Command: `python3 -c "print('v1.27.0')"`,
		}},
		Groups: []config.Group{{
			Name:    "db",
			Command: `python3 -c "open('ran','w').close()"`,
			Outputs: []string{"right.txt"},
			Tools:   []string{"sqlc"},
		}},
	}

	result, err := runConfig(cfg, "", commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupOK || group.Err != nil {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Tools) != 1 || group.Tools[0] != (ToolResult{Name: "sqlc", Have: "1.27.0"}) {
		t.Fatalf("tools = %+v", group.Tools)
	}
	if _, err := os.Stat(filepath.Join(root, "ran")); err != nil {
		t.Fatal(err)
	}
}

func TestOmittedCommandUsesNameVersion(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "right.txt", "ok\n")
	cfg := config.Config{
		Path:  filepath.Join(root, "genguard.yaml"),
		Tools: []config.Tool{{Name: "python3"}},
		Groups: []config.Group{{
			Name:    "db",
			Command: `python3 -c "open('ran','w').close()"`,
			Outputs: []string{"right.txt"},
			Tools:   []string{"python3"},
		}},
	}

	result, err := checkConfig(cfg, "", nil, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupOK || group.Err != nil {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Tools) != 1 || group.Tools[0].Name != "python3" || group.Tools[0].Want != "" || group.Tools[0].Have == "" {
		t.Fatalf("tools = %+v", group.Tools)
	}
	if _, err := os.Stat(filepath.Join(root, "ran")); err != nil {
		t.Fatal(err)
	}
}

func TestToolProbeFailuresLeaveTheTree(t *testing.T) {
	cases := []struct {
		name    string
		tool    config.Tool
		message string
	}{
		{
			name:    "missing",
			tool:    config.Tool{Name: "genguard-missing-tool", Version: "1.0.0"},
			message: "genguard-missing-tool: not on PATH",
		},
		{
			name: "exit 127",
			tool: config.Tool{
				Name:    "buf",
				Version: "1.0.0",
				Command: `python3 -c "import sys; sys.exit(127)"`,
			},
			message: "buf: not on PATH",
		},
		{
			name: "command failed",
			tool: config.Tool{
				Name:    "buf",
				Version: "1.0.0",
				Command: `python3 -c "import sys; sys.exit(3)"`,
			},
			message: "buf: version command failed (exit 3)",
		},
		{
			name: "no version",
			tool: config.Tool{
				Name:    "buf",
				Version: "1.0.0",
				Command: `python3 -c "print('nope')"`,
			},
			message: "buf: version command returned no version",
		},
		{
			name: "shorter version",
			tool: config.Tool{
				Name:    "buf",
				Version: "1.32",
				Command: `python3 -c "print('1.32.0')"`,
			},
			message: "buf: want 1.32, have 1.32.0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := gitRepo(t)
			writeTracked(t, root, "gen/out.txt", "keep\n")
			cfg := config.Config{
				Path:  filepath.Join(root, "genguard.yaml"),
				Tools: []config.Tool{tc.tool},
				Groups: []config.Group{{
					Name:    "protobuf",
					Command: `python3 -c "open('ran','w').close(); open('gen/out.txt','w').write('gone\n')"`,
					Outputs: []string{"gen/"},
					Clean:   true,
					Tools:   []string{tc.tool.Name},
				}},
			}
			result, err := checkConfig(cfg, "", nil, commandLog{})
			if err != nil {
				t.Fatal(err)
			}
			group := result.Groups[0]
			if group.Status != GroupError || group.Err == nil || group.Err.Error() != tc.message {
				t.Fatalf("group = %+v", group)
			}
			if result.ExitCode() != 2 {
				t.Fatalf("exit = %d", result.ExitCode())
			}
			assertFile(t, filepath.Join(root, "gen", "out.txt"), "keep\n")
			assertAbsent(t, filepath.Join(root, "ran"))
		})
	}
}

func TestToolProbesContinueAfterMismatch(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "gen/out.txt", "keep\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Tools: []config.Tool{
			{Name: "buf", Version: "1.32.0", Command: `python3 -c "print('1.0.0')"`},
			{Name: "sqlc", Version: "2.0.0", Command: `python3 -c "open('sqlc-probed','w').close(); print('9.9.9')"`},
		},
		Groups: []config.Group{{
			Name:    "protobuf",
			Command: `python3 -c "open('ran','w').close()"`,
			Outputs: []string{"gen/"},
			Clean:   true,
			Tools:   []string{"buf", "sqlc"},
		}},
	}

	result, err := checkConfig(cfg, "", nil, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "buf: want 1.32.0, have 1.0.0" {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Tools) != 2 {
		t.Fatalf("tools = %+v", group.Tools)
	}
	if group.Tools[1] != (ToolResult{Name: "sqlc", Want: "2.0.0", Have: "9.9.9"}) {
		t.Fatalf("sqlc = %+v", group.Tools[1])
	}
	if _, err := os.Stat(filepath.Join(root, "sqlc-probed")); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(root, "gen", "out.txt"), "keep\n")
	assertAbsent(t, filepath.Join(root, "ran"))
}

func TestSkippedGroupDoesNotProbe(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "queries/q.sql", "select 1;\n")
	writeTracked(t, root, "out.txt", "ok\n")
	writeTracked(t, root, "genguard.yaml", "groups: []\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Tools: []config.Tool{{
			Name:    "buf",
			Command: `python3 -c "open('probed','w').close(); print('1.0.0')"`,
		}},
		Groups: []config.Group{{
			Name:    "protobuf",
			Command: "true",
			Inputs:  []string{"queries/"},
			Outputs: []string{"out.txt"},
			Tools:   []string{"buf"},
		}},
	}

	result, err := checkConfig(cfg, "HEAD", nil, commandLog{})
	if err != nil {
		t.Fatal(err)
	}
	group := result.Groups[0]
	if group.Status != GroupSkipped || group.Tools != nil {
		t.Fatalf("group = %+v", group)
	}
	assertAbsent(t, filepath.Join(root, "probed"))
}

func TestToolProbeStaysOffVerboseLog(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "false")
	root := gitRepo(t)
	writeTracked(t, root, "gen/out.txt", "keep\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Tools: []config.Tool{{
			Name:    "buf",
			Version: "1.32.0",
			Command: `python3 -c "print('1.28.1')"`,
		}},
		Groups: []config.Group{{
			Name:    "protobuf",
			Command: `python3 -c "print('generator')"`,
			Outputs: []string{"gen/"},
			Tools:   []string{"buf"},
		}},
	}
	var buf bytes.Buffer
	result, err := checkConfig(cfg, "", nil, commandLog{w: &buf})
	if err != nil {
		t.Fatal(err)
	}
	if result.Groups[0].Status != GroupError {
		t.Fatalf("group = %+v", result.Groups[0])
	}
	if buf.Len() != 0 {
		t.Fatalf("log = %q", buf.String())
	}
}

func TestToolTimeoutLeavesOutputs(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "gen/out.txt", "keep\n")
	writeTracked(t, root, "right.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Tools: []config.Tool{{
			Name:    "buf",
			Version: "1.0.0",
			Command: `python3 -c "import time; time.sleep(5)"`,
		}},
		Groups: []config.Group{
			{
				Name:    "protobuf",
				Command: `python3 -c "open('ran','w').close()"`,
				Outputs: []string{"gen/"},
				Clean:   true,
				Tools:   []string{"buf"},
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
	group := result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "buf: command timed out after 200ms" {
		t.Fatalf("group = %+v", group)
	}
	assertFile(t, filepath.Join(root, "gen", "out.txt"), "keep\n")
	assertAbsent(t, filepath.Join(root, "ran"))
	if result.Groups[1].Status != GroupOK {
		t.Fatalf("other = %+v", result.Groups[1])
	}
}

func TestObservedVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{in: "buf 1.32.0\n", want: "1.32.0", ok: true},
		{in: "v1.32.0", want: "1.32.0", ok: true},
		{in: "V1.32.0", want: "1.32.0", ok: true},
		{in: "libprotoc 3.21.12", want: "3.21.12", ok: true},
		{in: "1.32", want: "1.32", ok: true},
		{in: "1.2.3-rc.1 is ready", want: "1.2.3-rc.1", ok: true},
		{in: "1.2.3-rc.1+build.5", want: "1.2.3-rc.1+build.5", ok: true},
		{in: "go version go1.22.0 darwin/arm64", want: "1.22.0", ok: true},
		{in: "1.0.0 then 2.0.0", want: "1.0.0", ok: true},
		{in: "nope", ok: false},
		{in: "", ok: false},
	}
	for _, tc := range cases {
		got, ok := observedVersion(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("observedVersion(%q) = %q, %v", tc.in, got, ok)
		}
	}
	if pinVersion("v1.32.0") != "1.32.0" || pinVersion("1.32") != "1.32" || pinVersion("") != "" || pinVersion("verbose") != "verbose" {
		t.Fatalf("pinVersion = %q %q %q %q", pinVersion("v1.32.0"), pinVersion("1.32"), pinVersion(""), pinVersion("verbose"))
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists: %v", path, err)
	}
}
