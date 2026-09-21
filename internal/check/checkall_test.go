package check_test

import (
	"os"
	"os/exec"
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
	configs, ok, drift, errorsN := run.Counts()
	if configs != 2 || ok != 1 || drift != 0 || errorsN != 0 {
		t.Fatalf("Counts = %d, %d, %d, %d", configs, ok, drift, errorsN)
	}
	if !strings.Contains(strings.Join(run.SummaryLines(), "\n"), "2 configs: 1 ok, 0 drift, 1 error") {
		t.Fatalf("summary = %v", run.SummaryLines())
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
	if run.RepoRoot != root {
		t.Fatalf("RepoRoot = %q, want %q", run.RepoRoot, root)
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
	if run.RepoRoot != root {
		t.Fatalf("RepoRoot = %q, want caller spelling %q", run.RepoRoot, root)
	}
	assertConfigPaths(t, run, api, top)
}

func TestCheckAllEmptyRepoRootUsesCwd(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)
	testutil.Chdir(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if run.RepoRoot != wd {
		t.Fatalf("RepoRoot = %q, want cwd %q", run.RepoRoot, wd)
	}
	assertSamePaths(t, run, api)
}

func TestCheckAllWhitespaceRepoRootUsesCwd(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)
	testutil.Chdir(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: " \t\n "})
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if run.RepoRoot != wd {
		t.Fatalf("RepoRoot = %q, want cwd %q", run.RepoRoot, wd)
	}
	assertSamePaths(t, run, api)
}

func TestCheckAllRelativeRepoRoot(t *testing.T) {
	root := initMonorepo(t)
	top := writeMiniConfig(t, root, "top", "true")
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)
	testutil.Chdir(t, filepath.Dir(root))

	relAPI := filepath.Join(filepath.Base(root), "api")
	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: relAPI})
	if err != nil {
		t.Fatal(err)
	}
	absAPI, err := filepath.Abs(relAPI)
	if err != nil {
		t.Fatal(err)
	}
	if run.RepoRoot != filepath.Dir(absAPI) {
		t.Fatalf("RepoRoot = %q, want %q", run.RepoRoot, filepath.Dir(absAPI))
	}
	assertSamePaths(t, run, api, top)
}

func TestCheckAllExplicitEmptyPathsDiscovers(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Paths: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	assertConfigPaths(t, run, api)
}

func TestCheckAllSymlinkRepoRootKeepsSpelling(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)
	link := filepath.Join(filepath.Dir(root), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: link})
	if err != nil {
		t.Fatal(err)
	}
	if run.RepoRoot != link {
		t.Fatalf("RepoRoot = %q, want symlink spelling %q", run.RepoRoot, link)
	}
	assertSamePaths(t, run, api)
}

func TestCheckAllSymlinkSubdirKeepsToplevelSpelling(t *testing.T) {
	root := initMonorepo(t)
	top := writeMiniConfig(t, root, "top", "true")
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)
	link := filepath.Join(filepath.Dir(root), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: filepath.Join(link, "api")})
	if err != nil {
		t.Fatal(err)
	}
	if run.RepoRoot != link {
		t.Fatalf("RepoRoot = %q, want symlink spelling %q", run.RepoRoot, link)
	}
	assertSamePaths(t, run, api, top)
}

func TestCheckAllSymlinkLeafUsesGitToplevelSpelling(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)
	shortcut := filepath.Join(filepath.Dir(root), "shortcut")
	if err := os.Symlink(filepath.Join(root, "api"), shortcut); err != nil {
		t.Fatal(err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: shortcut})
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", shortcut, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(strings.TrimSpace(string(out)))
	if run.RepoRoot != want {
		t.Fatalf("RepoRoot = %q, want git toplevel %q", run.RepoRoot, want)
	}
	if run.RepoRoot == shortcut {
		t.Fatalf("RepoRoot kept the leaf symlink %q", shortcut)
	}
	assertSamePaths(t, run, api)
}

func TestCheckAllDoesNotDedupSymlinkPaths(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)
	link := filepath.Join(root, "link-api")
	if err := os.Symlink(filepath.Join(root, "api"), link); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(link, "genguard.yaml")

	run, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{api, linked},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertConfigPaths(t, run, api, linked)
}

func TestCheckAllDedupsRelativeAndAbsolute(t *testing.T) {
	root := initMonorepo(t)
	writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)
	testutil.Chdir(t, root)

	rel := filepath.Join("api", "genguard.yaml")
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	run, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{rel, abs, rel},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertConfigPaths(t, run, abs)
}

func TestCheckAllSkipsVendorGitNodeModules(t *testing.T) {
	root := initMonorepo(t)
	for _, dir := range []string{
		filepath.Join(root, ".git", "hooks"),
		filepath.Join(root, "vendor", "lib"),
		filepath.Join(root, "web", "node_modules", "pkg"),
	} {
		writeMiniConfig(t, dir, "hidden", "true")
	}
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	assertConfigPaths(t, run, api)
}

func TestCheckAllDiscoversYamlAndYml(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	webDir := filepath.Join(root, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	web, err := testutil.WriteGenguardConfig(webDir, "", "", "genguard.yml", []testutil.GroupSpec{{
		Name:    "web",
		Command: "true",
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	assertConfigPaths(t, run, api, web)
}

func TestCheckAllMissingPathStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)
	missing := filepath.Join(root, "missing.yaml")

	run, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{missing, web},
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	missingRun := configByPath(t, run, missing)
	if missingRun.Err == nil {
		t.Fatal("expected load error for missing config")
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestCheckAllSubdirMissingAndUntracked(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	webDir := filepath.Join(root, "web")
	api := writeMiniConfig(t, apiDir, "api", "true")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	web, err := testutil.WriteGenguardConfig(webDir, "", "", "", []testutil.GroupSpec{{
		Name:    "web",
		Command: "true",
		Outputs: []string{"extra.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)
	if err := os.Remove(filepath.Join(apiDir, "out.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "extra.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 1 {
		t.Fatalf("exit = %d, want 1", run.ExitCode())
	}
	apiRun := configByPath(t, run, api)
	if len(apiRun.Result.Groups[0].Drifts) != 1 || apiRun.Result.Groups[0].Drifts[0] != (check.Drift{Group: "api", Path: "out.txt", Kind: "missing"}) {
		t.Fatalf("api drifts = %+v", apiRun.Result.Groups[0].Drifts)
	}
	webRun := configByPath(t, run, web)
	if len(webRun.Result.Groups[0].Drifts) != 1 || webRun.Result.Groups[0].Drifts[0] != (check.Drift{Group: "web", Path: "extra.txt", Kind: "untracked"}) {
		t.Fatalf("web drifts = %+v", webRun.Result.Groups[0].Drifts)
	}
}

func TestCheckAllCleanPassesAndStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: "printf 'ok\\n' > out.txt",
		Outputs: []string{"out.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(apiDir, "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	apiRun := configByPath(t, run, api)
	if apiRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("api status = %+v", apiRun.Result.Groups[0])
	}
	got, err := os.ReadFile(filepath.Join(apiDir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ok\n" {
		t.Fatalf("out.txt = %q", got)
	}
	webRun := configByPath(t, run, web)
	if webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestCheckAllCleanFailureStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: "true",
		Outputs: []string{"."},
		Clean:   true,
	}})
	if err != nil {
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
	apiRun := configByPath(t, run, api)
	if apiRun.Err != nil || apiRun.Result.Groups[0].Status != check.GroupError {
		t.Fatalf("api status = %+v, err = %v", apiRun.Result.Groups[0], apiRun.Err)
	}
	if apiRun.Result.Groups[0].Err == nil || !strings.Contains(apiRun.Result.Groups[0].Err.Error(), "clean refuses") {
		t.Fatalf("api error = %v", apiRun.Result.Groups[0].Err)
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestCheckAllDiscoveryWalkError(t *testing.T) {
	root := initMonorepo(t)
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(blocked, 0o755)
	})
	if f, err := os.Open(blocked); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced")
	}

	_, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err == nil {
		t.Fatal("expected discovery error")
	}
}

func TestCheckAllFormatsNestedDriftReport(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	writeMiniConfig(t, apiDir, "api", "printf 'new\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(apiDir, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	report, err := check.FormatRunFailureReport(run)
	if err != nil {
		t.Fatal(err)
	}
	apiHeader := filepath.Join("api", "genguard.yaml")
	webHeader := filepath.Join("web", "genguard.yaml")
	for _, want := range []string{
		apiHeader + "\n  api: drift (1 modified)",
		"1 group: 0 ok, 1 drift, 0 error\n\n" + webHeader,
		webHeader + "\n  web: OK",
		"1 group: 1 ok, 0 drift, 0 error\n\n2 configs: 1 ok, 1 drift, 0 error\n",
		"\nDrift\n" + apiHeader + "\n[modified] api: out.txt\n",
		"diff --git",
		"+new",
		"\nerror: 1 generated path drifted; commit the generator output or fix the command\n",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "\nDrift\n"+webHeader) {
		t.Fatalf("ok config listed under drift:\n%s", report)
	}
}

func TestCheckAllFormatsTwoDriftSections(t *testing.T) {
	root := initMonorepo(t)
	writeMiniConfig(t, filepath.Join(root, "api"), "api", "printf 'aaa\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(root, "api", "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeMiniConfig(t, filepath.Join(root, "web"), "web", "printf 'bbb\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(root, "web", "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	report, err := check.FormatRunFailureReport(run)
	if err != nil {
		t.Fatal(err)
	}
	apiHeader := filepath.Join("api", "genguard.yaml")
	webHeader := filepath.Join("web", "genguard.yaml")
	first := strings.Index(report, "\nDrift\n"+apiHeader+"\n[modified] api: out.txt\n")
	second := strings.Index(report, "\n\n"+webHeader+"\n[modified] web: out.txt\n")
	if first < 0 || second < 0 || first >= second {
		t.Fatalf("drift sections:\n%s", report)
	}
	if !strings.Contains(report, "+aaa") || !strings.Contains(report, "+bbb") {
		t.Fatalf("diffs missing:\n%s", report)
	}
	if !strings.Contains(report, "error: 2 generated paths drifted; commit the generator output or fix the command\n") {
		t.Fatalf("final line:\n%s", report)
	}
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
