package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/testutil"
)

func TestCheckAllAllOK(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, web)
	configs, ok, drift, errorsN := run.Counts()
	if configs != 2 || ok != 2 || drift != 0 || errorsN != 0 {
		t.Fatalf("Counts = %d, %d, %d, %d", configs, ok, drift, errorsN)
	}
}

func TestCheckAllDriftAndOK(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "printf 'new\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(root, "api", "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 1 {
		t.Fatalf("exit = %d, want 1", run.ExitCode())
	}
	assertConfigPaths(t, run, api, web)
	apiRun := configByPath(t, run, api)
	if len(apiRun.Result.Groups[0].Drifts) != 1 || apiRun.Result.Groups[0].Drifts[0] != (check.Drift{Group: "api", Path: "out.txt", Kind: "modified"}) {
		t.Fatalf("api drift = %+v", apiRun.Result.Groups[0].Drifts)
	}
	diff, err := check.DriftDiff(filepath.Dir(api), apiRun.Result.Groups[0].Drifts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "diff --git") {
		t.Fatalf("diff = %q", diff)
	}
	webRun := configByPath(t, run, web)
	if webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web status = %+v", webRun.Result.Groups[0])
	}
}

func TestCheckAllCommandErrorStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "exit 3")
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	assertConfigPaths(t, run, api, web)
	apiRun := configByPath(t, run, api)
	if apiRun.Result.Groups[0].Status != check.GroupError {
		t.Fatalf("api status = %+v", apiRun.Result.Groups[0])
	}
	webRun := configByPath(t, run, web)
	if webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestCheckAllInvalidYAMLStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	api := filepath.Join(apiDir, "genguard.yaml")
	if err := os.WriteFile(api, []byte("::::\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	assertConfigPaths(t, run, api, web)
	apiRun := configByPath(t, run, api)
	if apiRun.Err == nil {
		t.Fatal("expected load error for api")
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestCheckAllNestedConfigsBothRun(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	proto := writeMiniConfig(t, filepath.Join(root, "api", "proto"), "proto", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d", run.ExitCode())
	}
	assertConfigPaths(t, run, api, proto)
}

func TestCheckAllSortsAndDedupsPaths(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{web, api, web},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertConfigPaths(t, run, api, web)
}

func TestCheckAllRunsInSortedPathOrder(t *testing.T) {
	root := initMonorepo(t)
	writeMiniConfig(t, filepath.Join(root, "api"), "api", "printf a >> ../order.txt")
	writeMiniConfig(t, filepath.Join(root, "web"), "web", "printf b >> ../order.txt")
	commitAll(t, root)

	web := filepath.Join(root, "web", "genguard.yaml")
	api := filepath.Join(root, "api", "genguard.yaml")
	if _, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{web, api},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "order.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ab" {
		t.Fatalf("execution order = %q, want ab", got)
	}
}

func TestCheckAllEmptyDiscovery(t *testing.T) {
	root := initMonorepo(t)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Configs) != 0 {
		t.Fatalf("configs = %v, want empty", run.Configs)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, want 0", run.ExitCode())
	}
}

func TestCheckAllSetupErrorContinues(t *testing.T) {
	root := initMonorepo(t)
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	orphanDir := t.TempDir()
	orphan := writeMiniConfig(t, orphanDir, "orphan", "true")

	run, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{orphan, web},
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	if len(run.Configs) != 2 {
		t.Fatalf("configs = %d, want 2", len(run.Configs))
	}
	orphanRun := configByPath(t, run, orphan)
	if orphanRun.Err == nil {
		t.Fatal("expected git setup error for orphan config")
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestCheckAllMissingRepoRoot(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	_, err := check.CheckAll(check.CheckAllOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "git work tree") {
		t.Fatalf("error = %q", err)
	}
}

func TestCheckAllExplicitRepoRootMustBeGit(t *testing.T) {
	dir := t.TempDir()
	_, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: dir,
		Paths:    []string{filepath.Join(dir, "genguard.yaml")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "git work tree") {
		t.Fatalf("error = %q", err)
	}
}

func TestCheckAllRepoRootNormalizesToToplevel(t *testing.T) {
	root := initMonorepo(t)
	top := writeMiniConfig(t, root, "top", "true")
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: filepath.Join(root, "api")})
	if err != nil {
		t.Fatal(err)
	}
	if !samePath(t, run.RepoRoot, root) {
		t.Fatalf("RepoRoot = %q, want %q", run.RepoRoot, root)
	}
	assertSamePaths(t, run, api, top)
}

func initMonorepo(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeMiniConfig(t *testing.T, dir, name, command string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, err := testutil.WriteGenguardConfig(dir, "", "", "", []testutil.GroupSpec{{
		Name:    name,
		Command: command,
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func commitAll(t *testing.T, root string) {
	t.Helper()
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
}

func samePath(t *testing.T, got, want string) bool {
	t.Helper()
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	wantResolved, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatal(err)
	}
	return gotResolved == wantResolved
}

func assertSamePaths(t *testing.T, run check.RunResult, want ...string) {
	t.Helper()
	if len(run.Configs) != len(want) {
		t.Fatalf("configs = %d, want %d (%v)", len(run.Configs), len(want), runPaths(run))
	}
	for i, path := range want {
		if !samePath(t, run.Configs[i].Path, path) {
			t.Fatalf("configs[%d] = %q, want %q", i, run.Configs[i].Path, path)
		}
	}
}

func assertConfigPaths(t *testing.T, run check.RunResult, want ...string) {
	t.Helper()
	if len(run.Configs) != len(want) {
		t.Fatalf("configs = %d, want %d (%v)", len(run.Configs), len(want), runPaths(run))
	}
	for i, path := range want {
		if run.Configs[i].Path != path {
			t.Fatalf("configs[%d] = %q, want %q", i, run.Configs[i].Path, path)
		}
	}
}

func configByPath(t *testing.T, run check.RunResult, path string) check.ConfigRun {
	t.Helper()
	for _, cfg := range run.Configs {
		if cfg.Path == path {
			return cfg
		}
	}
	t.Fatalf("missing config %s in %v", path, runPaths(run))
	return check.ConfigRun{}
}

func runPaths(run check.RunResult) []string {
	paths := make([]string, len(run.Configs))
	for i, cfg := range run.Configs {
		paths[i] = cfg.Path
	}
	return paths
}
