package check

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wuddleko/genguard/internal/config"
)

func TestInterruptSkipsLaterClean(t *testing.T) {
	t.Run("check", func(t *testing.T) {
		assertLaterCleanSkipped(t, func(cfg config.Config, log commandLog) (ConfigResult, error) {
			return checkConfig(cfg, "", nil, log)
		})
	})
	t.Run("run", func(t *testing.T) {
		assertLaterCleanSkipped(t, func(cfg config.Config, log commandLog) (ConfigResult, error) {
			return runConfig(cfg, "", log)
		})
	})
}

func assertLaterCleanSkipped(t *testing.T, run func(config.Config, commandLog) (ConfigResult, error)) {
	t.Helper()
	root := gitRepo(t)
	writeTracked(t, root, "left.txt", "ok\n")
	writeTracked(t, root, "right.txt", "ok\n")
	ready := filepath.Join(root, "ready")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{
			{Name: "first", Command: napCommand(t, root, ready), Outputs: []string{"left.txt"}},
			{Name: "second", Command: "true", Outputs: []string{"right.txt"}, Clean: true},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	type got struct {
		result ConfigResult
		err    error
	}
	ch := make(chan got, 1)
	go func() {
		result, err := run(cfg, commandLog{ctx: ctx})
		ch <- got{result, err}
	}()
	cancelWhenReady(t, ready, cancel)

	var result got
	select {
	case result = <-ch:
	case <-time.After(10 * time.Second):
		t.Fatal("interrupt did not return")
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.result.ExitCode() != 2 {
		t.Fatalf("exit = %d", result.result.ExitCode())
	}
	if len(result.result.Groups) != 1 {
		t.Fatalf("groups = %+v", result.result.Groups)
	}
	first := result.result.Groups[0]
	if first.Name != "first" || first.Status != GroupError || first.Err == nil || first.Err.Error() != "interrupted" {
		t.Fatalf("first = %+v", first)
	}
	body, err := os.ReadFile(filepath.Join(root, "right.txt"))
	if err != nil || string(body) != "ok\n" {
		t.Fatalf("right.txt = %q, %v", body, err)
	}
}

func TestCanceledContextDoesNotClean(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "right.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "second",
			Command: "true",
			Outputs: []string{"right.txt"},
			Clean:   true,
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, name := range []string{"check", "run"} {
		t.Run(name, func(t *testing.T) {
			var result ConfigResult
			var err error
			if name == "check" {
				result, err = checkConfig(cfg, "", nil, commandLog{ctx: ctx})
			} else {
				result, err = runConfig(cfg, "", commandLog{ctx: ctx})
			}
			if err == nil || err.Error() != "interrupted" {
				t.Fatalf("err = %v", err)
			}
			if len(result.Groups) != 0 {
				t.Fatalf("groups = %+v", result.Groups)
			}
		})
	}
	got, err := os.ReadFile(filepath.Join(root, "right.txt"))
	if err != nil || string(got) != "ok\n" {
		t.Fatalf("right.txt = %q, %v", got, err)
	}
}

func TestCheckAllInterruptSkipsLaterConfig(t *testing.T) {
	for _, isolated := range []bool{false, true} {
		t.Run(isolatedName(isolated), func(t *testing.T) {
			root := gitRepo(t)
			ready := root + "-ready"
			api := filepath.Join(root, "api")
			web := filepath.Join(root, "web")
			if err := os.MkdirAll(api, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(web, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(api, "nap.py"), []byte(napScript(ready)), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(api, "out.txt"), []byte("ok\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(api, "genguard.yaml"), "groups:\n  - name: first\n    command: python3 nap.py\n    outputs:\n      - out.txt\n")
			writeFile(t, filepath.Join(web, "ran.py"), "open('ran','w').close()\n")
			writeFile(t, filepath.Join(web, "out.txt"), "ok\n")
			writeFile(t, filepath.Join(web, "genguard.yaml"), "groups:\n  - name: second\n    command: python3 ran.py\n    outputs:\n      - out.txt\n    clean: true\n")
			gitExec(t, root, "add", ".")
			gitExec(t, root, "commit", "-m", "seed")

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			type got struct {
				run RunResult
				err error
			}
			ch := make(chan got, 1)
			go func() {
				run, err := CheckAll(CheckAllOptions{
					RepoRoot: root,
					Paths:    []string{filepath.Join(api, "genguard.yaml"), filepath.Join(web, "genguard.yaml")},
					Isolated: isolated,
					Context:  ctx,
				})
				ch <- got{run, err}
			}()
			cancelWhenReady(t, ready, cancel)

			var result got
			select {
			case result = <-ch:
			case <-time.After(20 * time.Second):
				t.Fatal("interrupt did not return")
			}
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.run.ExitCode() != 2 || len(result.run.Configs) != 1 {
				t.Fatalf("exit = %d configs = %+v", result.run.ExitCode(), result.run.Configs)
			}
			cfg := result.run.Configs[0]
			if cfg.Err != nil || len(cfg.Result.Groups) != 1 || cfg.Result.Groups[0].Err == nil || cfg.Result.Groups[0].Err.Error() != "interrupted" {
				t.Fatalf("config = %+v", cfg)
			}
			text, err := os.ReadFile(filepath.Join(web, "out.txt"))
			if err != nil || string(text) != "ok\n" {
				t.Fatalf("web out.txt = %q, %v", text, err)
			}
			if _, err := os.Stat(filepath.Join(web, "ran")); !os.IsNotExist(err) {
				t.Fatalf("second config ran: %v", err)
			}
		})
	}
}

func TestInterruptCleanReportsWipe(t *testing.T) {
	for _, name := range []string{"check", "run"} {
		t.Run(name, func(t *testing.T) {
			root := gitRepo(t)
			writeTracked(t, root, "generated/hello.txt", "hello\n")
			ready := filepath.Join(root, "ready")
			cfg := config.Config{
				Path: filepath.Join(root, "genguard.yaml"),
				Groups: []config.Group{{
					Name:    "greeting",
					Command: `python3 -c "open('ready','w').close(); import time; time.sleep(30)"`,
					Outputs: []string{"generated/hello.txt"},
					Clean:   true,
				}},
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			ch := make(chan gotResult, 1)
			go func() {
				var result ConfigResult
				var err error
				if name == "check" {
					result, err = checkConfig(cfg, "", nil, commandLog{ctx: ctx})
				} else {
					result, err = runConfig(cfg, "", commandLog{ctx: ctx})
				}
				ch <- gotResult{result, err}
			}()
			cancelWhenReady(t, ready, cancel)

			result := waitResult(t, ch)
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.result.ExitCode() != 2 || len(result.result.Groups) != 1 {
				t.Fatalf("exit = %d groups = %+v", result.result.ExitCode(), result.result.Groups)
			}
			group := result.result.Groups[0]
			const want = "command failed after cleaning outputs: interrupted"
			if group.Status != GroupError || group.Err == nil || group.Err.Error() != want {
				t.Fatalf("group = %+v", group)
			}
			if _, statErr := os.Stat(filepath.Join(root, "generated", "hello.txt")); !os.IsNotExist(statErr) {
				t.Fatalf("wiped file stat = %v", statErr)
			}
		})
	}
}

func TestInterruptStillDiffs(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "generated/hello.txt", "old\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "greeting",
			Command: `python3 -c "f=open('generated/hello.txt','wb'); f.write(b'new\n'); f.flush(); f.close(); import time; time.sleep(30)"`,
			Outputs: []string{"generated/hello.txt"},
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := make(chan gotResult, 1)
	go func() {
		result, err := checkConfig(cfg, "", nil, commandLog{ctx: ctx})
		ch <- gotResult{result, err}
	}()
	cancelWhenContains(t, filepath.Join(root, "generated", "hello.txt"), "new\n", cancel)

	result := waitResult(t, ch)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.result.ExitCode() != 2 {
		t.Fatalf("exit = %d", result.result.ExitCode())
	}
	group := result.result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "interrupted" {
		t.Fatalf("group = %+v", group)
	}
	if len(group.Drifts) != 1 || group.Drifts[0].Kind != "modified" || group.Drifts[0].Path != "generated/hello.txt" {
		t.Fatalf("drifts = %+v", group.Drifts)
	}
	body, err := os.ReadFile(filepath.Join(root, "generated", "hello.txt"))
	if err != nil || string(body) != "new\n" {
		t.Fatalf("file = %q, %v", body, err)
	}
}

func TestInterruptBetweenGroupsKeepsDrift(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "left.txt", "old\n")
	writeTracked(t, root, "right.txt", "ok\n")
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{
			{Name: "first", Command: `python3 -c "open('left.txt','wb').write(b'new\n')"`, Outputs: []string{"left.txt"}},
			{Name: "second", Command: "true", Outputs: []string{"right.txt"}, Clean: true},
		},
	}
	ctx := &errGate{Context: context.Background(), path: filepath.Join(root, "left.txt")}
	result, err := checkConfig(cfg, "", nil, commandLog{ctx: ctx})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode() != 2 || len(result.Groups) != 1 {
		t.Fatalf("exit = %d groups = %+v", result.ExitCode(), result.Groups)
	}
	first := result.Groups[0]
	if first.Name != "first" || first.Status != GroupDrift || len(first.Drifts) != 1 || first.Drifts[0].Path != "left.txt" {
		t.Fatalf("first = %+v", first)
	}
	if result.cleanup == nil || result.cleanup.Error() != "interrupted" {
		t.Fatalf("cleanup = %v", result.cleanup)
	}
	body, err := os.ReadFile(filepath.Join(root, "right.txt"))
	if err != nil || string(body) != "ok\n" {
		t.Fatalf("right.txt = %q, %v", body, err)
	}
	report, reportErr := FormatFailureReport(result, root)
	if reportErr != nil {
		t.Fatal(reportErr)
	}
	if !strings.Contains(report, "left.txt") || !strings.Contains(report, "interrupted") {
		t.Fatalf("report = %s", report)
	}
}

func TestCanceledStopKeepsRunDrift(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run := RunResult{Configs: []ConfigRun{{
		Path: "api/genguard.yaml",
		Result: ConfigResult{Groups: []GroupResult{{
			Name:   "first",
			Status: GroupDrift,
			Drifts: []Drift{{Group: "first", Path: "out.txt", Kind: "modified"}},
		}}},
	}}}
	stop, err := canceledStop(ctx, run.ExitCode(), func(interrupt error) { noteInterruptedDrift(&run, interrupt) })
	if !stop || err != nil {
		t.Fatalf("stop = %v err = %v", stop, err)
	}
	if run.ExitCode() != 2 || run.Configs[0].Result.cleanup == nil || run.Configs[0].Result.cleanup.Error() != "interrupted" {
		t.Fatalf("exit = %d cleanup = %v", run.ExitCode(), run.Configs[0].Result.cleanup)
	}
	if run.Configs[0].Result.Groups[0].Status != GroupDrift {
		t.Fatalf("group = %+v", run.Configs[0].Result.Groups[0])
	}
}

func TestCanceledContextSkipsCheckAll(t *testing.T) {
	root := gitRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run, err := CheckAll(CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{filepath.Join(root, "genguard.yaml")},
		Context:  ctx,
	})
	if err == nil || err.Error() != "interrupted" || len(run.Configs) != 0 {
		t.Fatalf("err = %v configs = %+v", err, run.Configs)
	}
}

type gotResult struct {
	result ConfigResult
	err    error
}

func waitResult(t *testing.T, ch <-chan gotResult) gotResult {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(10 * time.Second):
		t.Fatal("interrupt did not return")
	}
	return gotResult{}
}

func cancelWhenContains(t *testing.T, path, want string, cancel context.CancelFunc) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		body, err := os.ReadFile(path)
		if err == nil && string(body) == want {
			cancel()
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("command did not write")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type errGate struct {
	context.Context
	path string
}

func (g *errGate) Err() error {
	body, err := os.ReadFile(g.path)
	if err == nil && string(body) == "new\n" {
		return context.Canceled
	}
	return nil
}

func (g *errGate) Done() <-chan struct{} { return nil }

func napCommand(t *testing.T, root, ready string) string {
	t.Helper()
	path := filepath.Join(root, "nap.py")
	if err := os.WriteFile(path, []byte(napScript(ready)), 0o644); err != nil {
		t.Fatal(err)
	}
	return "python3 nap.py"
}

func napScript(ready string) string {
	return "import time\nopen(" + strconv.Quote(ready) + ", 'w').close()\ntime.sleep(30)\n"
}

func cancelWhenReady(t *testing.T, ready string, cancel context.CancelFunc) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			cancel()
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("command did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func installHoldingGit(t *testing.T, mode, ready string) {
	t.Helper()
	saved := os.Getenv("PATH")
	installGitShim(t, mode)
	t.Setenv("PATH", os.Getenv("PATH")+string(os.PathListSeparator)+saved)
	t.Setenv("GENGUARD_GIT_HOLD_READY", ready)
}

func TestCancelDuringMergeBaseIsInterrupted(t *testing.T) {
	root := gitRepo(t)
	ready := filepath.Join(t.TempDir(), "ready")
	installHoldingGit(t, "hold-merge-base", ready)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := mergeBase(commandLog{ctx: ctx}, root, "HEAD")
		ch <- err
	}()
	cancelWhenReady(t, ready, cancel)

	var err error
	select {
	case err = <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("merge-base was not interrupted")
	}
	if time.Since(start) >= 8*time.Second {
		t.Fatalf("took %s", time.Since(start))
	}
	if err == nil || err.Error() != "interrupted" || strings.Contains(err.Error(), "bad --since") {
		t.Fatalf("err = %v", err)
	}
}

func TestCancelDuringDiffDoesNotExitZero(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "out.txt", "ok\n")
	ready := filepath.Join(t.TempDir(), "ready")
	installHoldingGit(t, "hold-diff", ready)
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "first",
			Command: "true",
			Outputs: []string{"out.txt"},
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := make(chan gotResult, 1)
	go func() {
		result, err := checkConfig(cfg, "", nil, commandLog{ctx: ctx})
		ch <- gotResult{result, err}
	}()
	cancelWhenReady(t, ready, cancel)

	result := waitResult(t, ch)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.result.ExitCode() != 2 || len(result.result.Groups) != 1 {
		t.Fatalf("exit = %d groups = %+v", result.result.ExitCode(), result.result.Groups)
	}
	group := result.result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "interrupted" {
		t.Fatalf("group = %+v", group)
	}
}

func TestCancelDuringAffectedCheckSkipsCommand(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "in.txt", "ok\n")
	writeTracked(t, root, "out.txt", "ok\n")
	ready := filepath.Join(t.TempDir(), "ready")
	installHoldingGit(t, "hold-diff", ready)
	cfg := config.Config{
		Path: filepath.Join(root, "genguard.yaml"),
		Groups: []config.Group{{
			Name:    "first",
			Command: "python3 -c \"open('ran','w').close()\"",
			Inputs:  []string{"in.txt"},
			Outputs: []string{"out.txt"},
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ch := make(chan gotResult, 1)
	go func() {
		result, err := checkConfig(cfg, "HEAD", nil, commandLog{ctx: ctx})
		ch <- gotResult{result, err}
	}()
	cancelWhenReady(t, ready, cancel)

	result := waitResult(t, ch)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.result.ExitCode() != 2 || len(result.result.Groups) != 1 {
		t.Fatalf("exit = %d groups = %+v", result.result.ExitCode(), result.result.Groups)
	}
	group := result.result.Groups[0]
	if group.Status != GroupError || group.Err == nil || group.Err.Error() != "interrupted" {
		t.Fatalf("group = %+v", group)
	}
	if _, err := os.Stat(filepath.Join(root, "ran")); !os.IsNotExist(err) {
		t.Fatalf("command ran: %v", err)
	}
}

func TestCancelDuringWorktreeAddIsInterrupted(t *testing.T) {
	root := gitRepo(t)
	writeTracked(t, root, "keep.txt", "ok\n")
	ready := filepath.Join(t.TempDir(), "ready")
	installHoldingGit(t, "hold-worktree", ready)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch := make(chan error, 1)
	go func() {
		_, err := addIsolatedWorktree(commandLog{ctx: ctx}, root)
		ch <- err
	}()
	cancelWhenReady(t, ready, cancel)

	var err error
	select {
	case err = <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("worktree add was not interrupted")
	}
	if err == nil || err.Error() != "interrupted" || strings.Contains(err.Error(), "git worktree add") {
		t.Fatalf("err = %v", err)
	}
}

func TestWorktreeRemoveFailureOutranksInterrupt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode does not block removal")
	}
	root := gitRepo(t)
	writeTracked(t, root, "keep.txt", "ok\n")
	t.Cleanup(func() { releaseWorktrees(t, root) })
	err := withIsolatedWorktree(commandLog{}, root, func(wt isolatedWorktree) error {
		if chmodErr := os.Chmod(wt.root, 0o555); chmodErr != nil {
			t.Fatal(chmodErr)
		}
		return errInterrupted
	})
	if err == nil || !strings.Contains(err.Error(), "git worktree remove") {
		t.Fatalf("err = %v", err)
	}
}

func isolatedName(isolated bool) string {
	if isolated {
		return "isolated"
	}
	return "checkout"
}
