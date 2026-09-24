package check_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/tests/testutil"
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

func TestCheckAllCleanFailureDoesNotBlameLaterConfig(t *testing.T) {
	root := initMonorepo(t)
	dir := filepath.Join(root, "api")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yamlPath, err := testutil.WriteGenguardConfig(dir, "", "", "genguard.yaml", []testutil.GroupSpec{{
		Name:    "yaml",
		Command: "exit 3",
		Outputs: []string{"out.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	ymlPath, err := testutil.WriteGenguardConfig(dir, "", "", "genguard.yml", []testutil.GroupSpec{{
		Name:    "yml",
		Command: "true",
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)

	_, err = check.CheckAll(check.CheckAllOptions{RepoRoot: root})
	if err == nil || !strings.Contains(err.Error(), "both genguard.yaml and genguard.yml") {
		t.Fatalf("discovery error = %v", err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Paths: []string{yamlPath, ymlPath}})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	yamlRun := configByPath(t, run, yamlPath)
	if yamlRun.Result.Groups[0].Status != check.GroupError {
		t.Fatalf("yaml status = %+v", yamlRun.Result.Groups[0])
	}
	if yamlRun.Result.Groups[0].Err == nil || !strings.Contains(yamlRun.Result.Groups[0].Err.Error(), "after cleaning outputs") {
		t.Fatalf("yaml error = %v", yamlRun.Result.Groups[0].Err)
	}
	ymlRun := configByPath(t, run, ymlPath)
	if ymlRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("yml status = %+v", ymlRun.Result.Groups[0])
	}
	if _, statErr := os.Stat(filepath.Join(dir, "out.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("out.txt should have been wiped: %v", statErr)
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

func TestCheckAllIsolatedOverlappingCleanDoesNotShareWipe(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	nestedDir := filepath.Join(apiDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: "exit 1",
		Outputs: []string{"nested/out.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := testutil.WriteGenguardConfig(nestedDir, "", "", "", []testutil.GroupSpec{{
		Name:    "nested",
		Command: "true",
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(nestedDir, "out.txt")
	if err := os.WriteFile(out, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)
	if err := os.WriteFile(out, []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, nested)
	apiRun := configByPath(t, run, api)
	if apiRun.Result.Groups[0].Status != check.GroupError {
		t.Fatalf("api status = %+v", apiRun.Result.Groups[0])
	}
	nestedRun := configByPath(t, run, nested)
	if nestedRun.Err != nil || nestedRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("nested saw the other wipe: %+v", nestedRun)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "dirty\n" {
		t.Fatalf("user out.txt = %q, isolated --all wrote the checkout", got)
	}
}

func TestCheckAllIsolatedDiscoversHeadConfigs(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "true")
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	vendor := writeMiniConfig(t, filepath.Join(root, "vendor", "lib"), "vendor", "true")
	commitAll(t, root)

	extra := writeMiniConfig(t, filepath.Join(root, "extra"), "extra", "true")
	if err := os.WriteFile(filepath.Join(root, "api", "genguard.yml"), []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(web); err != nil {
		t.Fatal(err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, web)
	for _, cfg := range run.Configs {
		if cfg.Path == vendor || cfg.Path == extra {
			t.Fatalf("discovered %s", cfg.Path)
		}
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("deleted checkout config did not run from HEAD: %+v", webRun)
	}
	if _, err := os.Stat(web); !os.IsNotExist(err) {
		t.Fatal("isolated --all restored the deleted config")
	}
}

func TestCheckAllIsolatedSkipsUntrackedOnly(t *testing.T) {
	root := initMonorepo(t)
	if err := os.WriteFile(filepath.Join(root, "README"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)
	writeMiniConfig(t, filepath.Join(root, "extra"), "extra", "true")

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Configs) != 0 {
		t.Fatalf("configs = %+v, want none", run.Configs)
	}
}

func TestCheckAllIsolatedBothNamesAtHeadIsFatal(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	writeMiniConfig(t, apiDir, "api", "true")
	if err := os.WriteFile(filepath.Join(apiDir, "genguard.yml"), []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)

	_, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true})
	if err == nil || !strings.Contains(err.Error(), "both genguard.yaml and genguard.yml") {
		t.Fatalf("err = %v", err)
	}
}

func TestCheckAllIsolatedExplicitPathNotInHead(t *testing.T) {
	root := initMonorepo(t)
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)
	extra := writeMiniConfig(t, filepath.Join(root, "extra"), "extra", "true")

	run, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{extra, web},
		Isolated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	extraRun := configByPath(t, run, extra)
	if extraRun.Err == nil || !strings.Contains(extraRun.Err.Error(), "is not in HEAD") || strings.Contains(extraRun.Err.Error(), "genguard-") {
		t.Fatalf("extra err = %v", extraRun.Err)
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestCheckAllIsolatedLoadErrorStillRunsOthers(t *testing.T) {
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

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true})
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
	if strings.Contains(apiRun.Path, "genguard-") || strings.Contains(apiRun.Err.Error(), "genguard-") {
		t.Fatalf("worktree path leaked: path=%q err=%v", apiRun.Path, apiRun.Err)
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestCheckAllIsolatedSinceUsesOneMergeBase(t *testing.T) {
	root := writeSinceRepo(t)
	if err := os.WriteFile(filepath.Join(root, "queries", "q.sql"), []byte("select 9;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true, Since: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	if groupInRun(t, run, "sqlc").Status != check.GroupSkipped {
		t.Fatalf("sqlc = %+v", groupInRun(t, run, "sqlc"))
	}
	if groupInRun(t, run, "protobuf").Status != check.GroupSkipped {
		t.Fatalf("protobuf = %+v", groupInRun(t, run, "protobuf"))
	}
	if groupInRun(t, run, "plain").Status != check.GroupOK {
		t.Fatalf("plain = %+v", groupInRun(t, run, "plain"))
	}
	if groupInRun(t, run, "api").Status != check.GroupSkipped {
		t.Fatalf("api = %+v", groupInRun(t, run, "api"))
	}
	if markerExists(root, "sqlc-ran") || markerExists(root, "plain-ran") || markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("isolated --all wrote a marker into the user tree")
	}

	commitPath(t, root, "queries/q.sql", "select 2;\n")
	run, err = check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true, Since: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d after commit", run.ExitCode())
	}
	if groupInRun(t, run, "sqlc").Status != check.GroupOK {
		t.Fatalf("sqlc after commit = %+v", groupInRun(t, run, "sqlc"))
	}
	if groupInRun(t, run, "protobuf").Status != check.GroupSkipped {
		t.Fatalf("protobuf after commit = %+v", groupInRun(t, run, "protobuf"))
	}
	if markerExists(root, "sqlc-ran") {
		t.Fatal("isolated command ran in the user tree")
	}
}

func TestCheckAllIsolatedBadSinceIsFatal(t *testing.T) {
	root := writeSinceRepo(t)
	_, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true, Since: "not-a-ref"})
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("err = %v", err)
	}
	if markerExists(root, "plain-ran") || markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("bad ref ran a group")
	}
}

func TestCheckAllIsolatedReportUsesCapturedDiff(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	writeMiniConfig(t, apiDir, "api", "printf 'new\\n' > out.txt")
	if err := os.WriteFile(filepath.Join(apiDir, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)
	if err := os.WriteFile(filepath.Join(apiDir, "out.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run, err := check.CheckAll(check.CheckAllOptions{RepoRoot: root, Isolated: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 1 {
		t.Fatalf("exit = %d, want 1", run.ExitCode())
	}
	report, err := check.FormatRunFailureReport(run)
	if err != nil {
		t.Fatal(err)
	}
	apiHeader := filepath.Join("api", "genguard.yaml")
	if !strings.Contains(report, apiHeader+"\n[modified] api: out.txt\n") {
		t.Fatalf("report missing user config path:\n%s", report)
	}
	if strings.Contains(report, "genguard-") {
		t.Fatalf("worktree path leaked:\n%s", report)
	}
	if !strings.Contains(report, "+new") || strings.Contains(report, "dirty") {
		t.Fatalf("diff used the user tree:\n%s", report)
	}
	got, err := os.ReadFile(filepath.Join(apiDir, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "dirty\n" {
		t.Fatalf("user out.txt = %q", got)
	}
}

func TestCheckAllIsolatedWorktreeAddFailureStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "true")
	commitAll(t, root)

	orphanDir := t.TempDir()
	if err := testutil.InitGitRepo(orphanDir); err != nil {
		t.Fatal(err)
	}
	orphan := writeMiniConfig(t, orphanDir, "orphan", "true")

	run, err := check.CheckAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{orphan, web},
		Isolated: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	orphanRun := configByPath(t, run, orphan)
	if orphanRun.Err == nil || !strings.Contains(orphanRun.Err.Error(), "git worktree add:") {
		t.Fatalf("orphan err = %v", orphanRun.Err)
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestRunAllWritesEveryConfig(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", `python3 -c "open('api-ran','w').close()"`)
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", `python3 -c "open('web-ran','w').close()"`)
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, web)
	if !markerExists(filepath.Join(root, "api"), "api-ran") || !markerExists(filepath.Join(root, "web"), "web-ran") {
		t.Fatal("a config did not run")
	}
}

func TestRunAllCommandErrorStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "exit 3")
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", `python3 -c "open('web-ran','w').close()"`)
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
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
	if webRun.Result.Groups[0].Status != check.GroupOK || !markerExists(filepath.Join(root, "web"), "web-ran") {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestRunAllInvalidYAMLStillRunsOthers(t *testing.T) {
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

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	apiRun := configByPath(t, run, api)
	if apiRun.Err == nil {
		t.Fatal("expected load error for api")
	}
	webRun := configByPath(t, run, web)
	if webRun.Err != nil || webRun.Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("web did not run: %+v", webRun)
	}
}

func TestRunAllOverlappingCleanSharesTree(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	nestedDir := filepath.Join(apiDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: "exit 1",
		Outputs: []string{"nested/out.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := testutil.WriteGenguardConfig(nestedDir, "", "", "", []testutil.GroupSpec{{
		Name:    "nested",
		Command: "test ! -f out.txt",
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(nestedDir, "out.txt")
	if err := os.WriteFile(out, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	assertConfigPaths(t, run, api, nested)
	if configByPath(t, run, api).Result.Groups[0].Status != check.GroupError {
		t.Fatal("api clean command should fail")
	}
	if configByPath(t, run, nested).Result.Groups[0].Status != check.GroupOK {
		t.Fatal("nested command should still run")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("shared tree kept %s: %v", out, err)
	}
}

func TestRunAllSinceSelectsPerGroup(t *testing.T) {
	root := writeSinceRepo(t)
	commitPath(t, root, "queries/q.sql", "select 2;\n")

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Since: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d", run.ExitCode())
	}
	sqlc := groupInRun(t, run, "sqlc")
	protobuf := groupInRun(t, run, "protobuf")
	plain := groupInRun(t, run, "plain")
	api := groupInRun(t, run, "api")
	if sqlc.Status != check.GroupOK || protobuf.Status != check.GroupSkipped || plain.Status != check.GroupOK || api.Status != check.GroupSkipped {
		t.Fatalf("sqlc=%s protobuf=%s plain=%s api=%s", sqlc.Status, protobuf.Status, plain.Status, api.Status)
	}
	if !markerExists(root, "sqlc-ran") || !markerExists(root, "plain-ran") || markerExists(root, "proto-ran") || markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("run --all did not select per group")
	}
	got, err := os.ReadFile(filepath.Join(root, "gen", "a.pb.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package gen\n" {
		t.Fatalf("clean ran on a skipped group: %q", got)
	}
}

func TestRunAllBadSinceRunsNothing(t *testing.T) {
	root := writeSinceRepo(t)
	_, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Since: "not-a-ref"})
	if err == nil || !strings.Contains(err.Error(), "bad --since ref") {
		t.Fatalf("err = %v", err)
	}
	if markerExists(root, "plain-ran") || markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("bad ref ran a group")
	}
}

func TestRunAllRejectsIsolated(t *testing.T) {
	_, err := check.RunAll(check.CheckAllOptions{Isolated: true})
	if err == nil || !strings.Contains(err.Error(), "--isolated is not valid") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunAllCleanSuccessLaterCommandSeesWipe(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	nestedDir := filepath.Join(apiDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: "true",
		Outputs: []string{"nested/out.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := testutil.WriteGenguardConfig(nestedDir, "", "", "", []testutil.GroupSpec{{
		Name:    "nested",
		Command: "test ! -f out.txt",
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(nestedDir, "out.txt")
	if err := os.WriteFile(out, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, nested)
	if configByPath(t, run, api).Result.Groups[0].Status != check.GroupOK {
		t.Fatal("api clean should succeed")
	}
	if configByPath(t, run, nested).Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("nested did not see the wipe: %+v", configByPath(t, run, nested).Result.Groups[0])
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("shared tree kept %s: %v", out, err)
	}
}

func TestRunAllLaterCommandSeesEarlierWrite(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	nestedDir := filepath.Join(apiDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(nestedDir, "note.txt")
	if err := os.WriteFile(note, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: `python3 -c "open('nested/note.txt','wb').write(b'from-api\n')"`,
		Outputs: []string{"nested/note.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := testutil.WriteGenguardConfig(nestedDir, "", "", "", []testutil.GroupSpec{{
		Name:    "nested",
		Command: `python3 -c "import pathlib; assert pathlib.Path('note.txt').read_bytes()==b'from-api\n'"`,
		Outputs: []string{"note.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, nested)
	if configByPath(t, run, nested).Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("nested did not see the earlier write: %+v", configByPath(t, run, nested).Result.Groups[0])
	}
	got, err := os.ReadFile(note)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "from-api\n" {
		t.Fatalf("note.txt = %q", got)
	}
}

func TestRunAllSinceCleanedLiteralOutputRunsLaterConfig(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	nestedDir := filepath.Join(apiDir, "nested")
	if err := os.MkdirAll(filepath.Join(nestedDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "src", "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(nestedDir, "out.txt")
	if err := os.WriteFile(out, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: "true",
		Outputs: []string{"nested/out.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := testutil.WriteGenguardConfig(nestedDir, "", "", "", []testutil.GroupSpec{{
		Name:    "nested",
		Command: `test ! -f out.txt && python3 -c "open('ran','w').close()"`,
		Inputs:  []string{"src/"},
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	commitBase(t, root)

	alone, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Since: "base", Paths: []string{nested}})
	if err != nil {
		t.Fatal(err)
	}
	if alone.ExitCode() != 0 || groupInRun(t, alone, "nested").Status != check.GroupSkipped {
		t.Fatalf("unchanged nested = %+v", alone.Configs)
	}
	if markerExists(nestedDir, "ran") {
		t.Fatal("unchanged nested ran")
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Since: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, nested)
	if configByPath(t, run, api).Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("api = %+v", configByPath(t, run, api).Result.Groups[0])
	}
	if groupInRun(t, run, "nested").Status != check.GroupOK || !markerExists(nestedDir, "ran") {
		t.Fatalf("deleted literal output did not run nested: %+v", groupInRun(t, run, "nested"))
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("out.txt = %v", err)
	}
}

func TestRunAllSinceCleanedTrackedOutputRunsDirectoryAndGlob(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	nestedDir := filepath.Join(apiDir, "nested")
	genDir := filepath.Join(nestedDir, "gen")
	if err := os.MkdirAll(filepath.Join(nestedDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "src", "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kept := filepath.Join(genDir, "kept.txt")
	if err := os.WriteFile(kept, []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: "true",
		Outputs: []string{"nested/gen/kept.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := testutil.WriteGenguardConfig(nestedDir, "", "", "", []testutil.GroupSpec{
		{
			Name:    "dir",
			Command: `test ! -f gen/kept.txt && python3 -c "open('dir-ran','w').close()"`,
			Inputs:  []string{"src/"},
			Outputs: []string{"gen/"},
		},
		{
			Name:    "glob",
			Command: `test ! -f gen/kept.txt && python3 -c "open('glob-ran','w').close()"`,
			Inputs:  []string{"src/"},
			Outputs: []string{"gen/*.txt"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commitBase(t, root)

	alone, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Since: "base", Paths: []string{nested}})
	if err != nil {
		t.Fatal(err)
	}
	if alone.ExitCode() != 0 || groupInRun(t, alone, "dir").Status != check.GroupSkipped || groupInRun(t, alone, "glob").Status != check.GroupSkipped {
		t.Fatalf("unchanged nested = %+v", alone.Configs)
	}
	if markerExists(nestedDir, "dir-ran") || markerExists(nestedDir, "glob-ran") {
		t.Fatal("unchanged nested ran")
	}

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Since: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, nested)
	if groupInRun(t, run, "dir").Status != check.GroupOK || groupInRun(t, run, "glob").Status != check.GroupOK {
		t.Fatalf("dir=%+v glob=%+v", groupInRun(t, run, "dir"), groupInRun(t, run, "glob"))
	}
	if !markerExists(nestedDir, "dir-ran") || !markerExists(nestedDir, "glob-ran") {
		t.Fatal("tracked deletion did not run the later groups")
	}
	if _, err := os.Stat(kept); !os.IsNotExist(err) {
		t.Fatalf("kept.txt = %v", err)
	}
}

func TestRunAllSinceCleanedUntrackedFileLeavesDirectoryAndGlobSkipped(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	nestedDir := filepath.Join(apiDir, "nested")
	if err := os.MkdirAll(filepath.Join(nestedDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "src", "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: "true",
		Outputs: []string{"nested/gen/extra.txt"},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := testutil.WriteGenguardConfig(nestedDir, "", "", "", []testutil.GroupSpec{
		{
			Name:    "dir",
			Command: `python3 -c "open('dir-ran','w').close()"`,
			Inputs:  []string{"src/"},
			Outputs: []string{"gen/"},
		},
		{
			Name:    "glob",
			Command: `python3 -c "open('glob-ran','w').close()"`,
			Inputs:  []string{"src/"},
			Outputs: []string{"gen/*.txt"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	commitBase(t, root)

	alone, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Since: "base", Paths: []string{nested}})
	if err != nil {
		t.Fatal(err)
	}
	if alone.ExitCode() != 0 || groupInRun(t, alone, "dir").Status != check.GroupSkipped || groupInRun(t, alone, "glob").Status != check.GroupSkipped {
		t.Fatalf("unchanged nested = %+v", alone.Configs)
	}

	extra := filepath.Join(nestedDir, "gen", "extra.txt")
	if err := os.MkdirAll(filepath.Dir(extra), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Since: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, nested)
	if configByPath(t, run, api).Result.Groups[0].Status != check.GroupOK {
		t.Fatalf("api = %+v", configByPath(t, run, api).Result.Groups[0])
	}
	if groupInRun(t, run, "dir").Status != check.GroupSkipped || groupInRun(t, run, "glob").Status != check.GroupSkipped {
		t.Fatalf("dir=%+v glob=%+v", groupInRun(t, run, "dir"), groupInRun(t, run, "glob"))
	}
	if markerExists(nestedDir, "dir-ran") || markerExists(nestedDir, "glob-ran") {
		t.Fatal("untracked deletion ran a later group")
	}
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatalf("extra.txt = %v", err)
	}
}

func TestRunAllSkipsVendorGitNodeModules(t *testing.T) {
	root := initMonorepo(t)
	for _, dir := range []string{
		filepath.Join(root, ".git", "hooks"),
		filepath.Join(root, "vendor", "lib"),
		filepath.Join(root, "web", "node_modules", "pkg"),
	} {
		writeMiniConfig(t, dir, "hidden", `python3 -c "open('ran','w').close()"`)
	}
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", `python3 -c "open('api-ran','w').close()"`)
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api)
	if !markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("api did not run")
	}
	for _, dir := range []string{
		filepath.Join(root, ".git", "hooks"),
		filepath.Join(root, "vendor", "lib"),
		filepath.Join(root, "web", "node_modules", "pkg"),
	} {
		if markerExists(dir, "ran") {
			t.Fatalf("ran a skipped config in %s", dir)
		}
	}
}

func TestRunAllDiscoversAndRunsYml(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", `python3 -c "open('api-ran','w').close()"`)
	webDir := filepath.Join(root, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	web, err := testutil.WriteGenguardConfig(webDir, "", "", "genguard.yml", []testutil.GroupSpec{{
		Name:    "web",
		Command: `python3 -c "open('web-ran','w').close()"`,
		Outputs: []string{"out.txt"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "out.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, web)
	if !markerExists(filepath.Join(root, "api"), "api-ran") || !markerExists(webDir, "web-ran") {
		t.Fatal("yaml or yml config did not run")
	}
}

func TestRunAllBothNamesRunsNothing(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	writeMiniConfig(t, apiDir, "yaml", `python3 -c "open('yaml-ran','w').close()"`)
	if _, err := testutil.WriteGenguardConfig(apiDir, "", "", "genguard.yml", []testutil.GroupSpec{{
		Name:    "yml",
		Command: `python3 -c "open('yml-ran','w').close()"`,
		Outputs: []string{"other.txt"},
	}}); err != nil {
		t.Fatal(err)
	}
	writeMiniConfig(t, filepath.Join(root, "web"), "web", `python3 -c "open('web-ran','w').close()"`)
	commitAll(t, root)

	_, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err == nil || !strings.Contains(err.Error(), "both genguard.yaml and genguard.yml") || !strings.Contains(err.Error(), apiDir) {
		t.Fatalf("err = %v", err)
	}
	if markerExists(apiDir, "yaml-ran") || markerExists(apiDir, "yml-ran") || markerExists(filepath.Join(root, "web"), "web-ran") {
		t.Fatal("both names ran a command")
	}
}

func TestRunAllUntrackedConfigRuns(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", `python3 -c "open('api-ran','w').close()"`)
	commitAll(t, root)
	extra := writeMiniConfig(t, filepath.Join(root, "extra"), "extra", `python3 -c "open('extra-ran','w').close()"`)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, extra)
	if !markerExists(filepath.Join(root, "api"), "api-ran") || !markerExists(filepath.Join(root, "extra"), "extra-ran") {
		t.Fatal("untracked config did not run")
	}
}

func TestRunAllIgnoredDirectoryStillRuns(t *testing.T) {
	root := initMonorepo(t)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", `python3 -c "open('api-ran','w').close()"`)
	commitAll(t, root)
	ignored := writeMiniConfig(t, filepath.Join(root, "ignored"), "ignored", `python3 -c "open('ignored-ran','w').close()"`)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, ignored)
	if !markerExists(filepath.Join(root, "ignored"), "ignored-ran") {
		t.Fatal("ignored directory did not run")
	}
}

func TestRunAllDoesNotRunConfigDeletedFromCheckout(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", `python3 -c "open('api-ran','w').close()"`)
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", `python3 -c "open('web-ran','w').close()"`)
	commitAll(t, root)
	if err := os.Remove(web); err != nil {
		t.Fatal(err)
	}

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api)
	if !markerExists(filepath.Join(root, "api"), "api-ran") || markerExists(filepath.Join(root, "web"), "web-ran") {
		t.Fatal("deleted checkout config ran")
	}
	if _, err := os.Stat(web); !os.IsNotExist(err) {
		t.Fatal("deleted config was restored")
	}
}

func TestRunAllSortsDedupsAndRunsInPathOrder(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", "printf a >> ../order.txt")
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", "printf b >> ../order.txt")
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{web, api, web},
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	assertConfigPaths(t, run, api, web)
	got, err := os.ReadFile(filepath.Join(root, "order.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ab" {
		t.Fatalf("execution order = %q, want ab", got)
	}
}

func TestRunAllRunsInDiscoveredPathOrder(t *testing.T) {
	root := initMonorepo(t)
	writeMiniConfig(t, filepath.Join(root, "z"), "z", "printf z >> ../order.txt")
	writeMiniConfig(t, filepath.Join(root, "a"), "a", "printf a >> ../order.txt")
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 0 {
		t.Fatalf("exit = %d, configs = %+v", run.ExitCode(), run.Configs)
	}
	got, err := os.ReadFile(filepath.Join(root, "order.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "az" {
		t.Fatalf("execution order = %q, want az", got)
	}
}

func TestRunAllMissingPathStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", `python3 -c "open('web-ran','w').close()"`)
	commitAll(t, root)
	missing := filepath.Join(root, "missing.yaml")

	run, err := check.RunAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{missing, web},
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	if configByPath(t, run, missing).Err == nil {
		t.Fatal("expected load error for missing config")
	}
	if !markerExists(filepath.Join(root, "web"), "web-ran") {
		t.Fatal("web did not run")
	}
}

func TestRunAllOutsideRepoStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", `python3 -c "open('web-ran','w').close()"`)
	commitAll(t, root)
	orphanDir := t.TempDir()
	orphan := writeMiniConfig(t, orphanDir, "orphan", `python3 -c "open('ran','w').close()"`)

	run, err := check.RunAll(check.CheckAllOptions{
		RepoRoot: root,
		Paths:    []string{orphan, web},
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	orphanRun := configByPath(t, run, orphan)
	if orphanRun.Err == nil || !strings.Contains(orphanRun.Err.Error(), "git work tree") {
		t.Fatalf("orphan = %+v", orphanRun)
	}
	if markerExists(orphanDir, "ran") || !markerExists(filepath.Join(root, "web"), "web-ran") {
		t.Fatal("outside repo changed which configs ran")
	}
}

func TestRunAllCleanRefusalStillRunsOthers(t *testing.T) {
	root := initMonorepo(t)
	apiDir := filepath.Join(root, "api")
	if err := os.MkdirAll(apiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	api, err := testutil.WriteGenguardConfig(apiDir, "", "", "", []testutil.GroupSpec{{
		Name:    "api",
		Command: `python3 -c "open('api-ran','w').close()"`,
		Outputs: []string{"."},
		Clean:   true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	web := writeMiniConfig(t, filepath.Join(root, "web"), "web", `python3 -c "open('web-ran','w').close()"`)
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode() != 2 {
		t.Fatalf("exit = %d, want 2", run.ExitCode())
	}
	assertConfigPaths(t, run, api, web)
	apiRun := configByPath(t, run, api)
	if apiRun.Err != nil || apiRun.Result.Groups[0].Status != check.GroupError {
		t.Fatalf("api = %+v, err = %v", apiRun.Result.Groups[0], apiRun.Err)
	}
	if apiRun.Result.Groups[0].Err == nil || !strings.Contains(apiRun.Result.Groups[0].Err.Error(), "clean refuses") {
		t.Fatalf("api error = %v", apiRun.Result.Groups[0].Err)
	}
	if markerExists(apiDir, "api-ran") || !markerExists(filepath.Join(root, "web"), "web-ran") {
		t.Fatal("refused clean ran its command or skipped web")
	}
}

func TestRunAllEmptyDiscovery(t *testing.T) {
	root := initMonorepo(t)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Configs) != 0 || run.ExitCode() != 0 {
		t.Fatalf("configs = %+v, exit = %d", run.Configs, run.ExitCode())
	}
}

func TestRunAllExplicitEmptyPathsDiscovers(t *testing.T) {
	root := initMonorepo(t)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", `python3 -c "open('api-ran','w').close()"`)
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: root, Paths: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	assertConfigPaths(t, run, api)
	if !markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("empty paths did not discover")
	}
}

func TestRunAllRepoRootNormalizesToToplevel(t *testing.T) {
	root := initMonorepo(t)
	top := writeMiniConfig(t, root, "top", `python3 -c "open('top-ran','w').close()"`)
	api := writeMiniConfig(t, filepath.Join(root, "api"), "api", `python3 -c "open('api-ran','w').close()"`)
	commitAll(t, root)

	run, err := check.RunAll(check.CheckAllOptions{RepoRoot: filepath.Join(root, "api")})
	if err != nil {
		t.Fatal(err)
	}
	if run.RepoRoot != root {
		t.Fatalf("RepoRoot = %q, want %q", run.RepoRoot, root)
	}
	assertConfigPaths(t, run, api, top)
	if !markerExists(root, "top-ran") || !markerExists(filepath.Join(root, "api"), "api-ran") {
		t.Fatal("subdir root did not run every config")
	}
}

func TestRunAllMissingRepoRoot(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	_, err := check.RunAll(check.CheckAllOptions{})
	if err == nil || !strings.Contains(err.Error(), "git work tree") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunAllExplicitRepoRootMustBeGit(t *testing.T) {
	dir := t.TempDir()
	path := writeMiniConfig(t, dir, "lone", `python3 -c "open('ran','w').close()"`)
	_, err := check.RunAll(check.CheckAllOptions{
		RepoRoot: dir,
		Paths:    []string{path},
	})
	if err == nil || !strings.Contains(err.Error(), "git work tree") {
		t.Fatalf("err = %v", err)
	}
	if markerExists(dir, "ran") {
		t.Fatal("non-repo ran a command")
	}
}

func TestRunAllDiscoveryWalkError(t *testing.T) {
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

	_, err := check.RunAll(check.CheckAllOptions{RepoRoot: root})
	if err == nil {
		t.Fatal("expected discovery error")
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

func commitBase(t *testing.T, root string) {
	t.Helper()
	commitAll(t, root)
	if err := testutil.Git(root, "branch", "base"); err != nil {
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
