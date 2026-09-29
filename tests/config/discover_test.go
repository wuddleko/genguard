package config_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/internal/discover"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestFindAllNestedYamlAndYml(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	web := filepath.Join(root, "web")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "genguard.yaml", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(web, "gen/", "", "genguard.yml", nil); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(api, "genguard.yaml"),
		filepath.Join(web, "genguard.yml"),
	}
	assertPaths(t, found, want)
}

func TestFindAllIncludesNestedConfigs(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	proto := filepath.Join(api, "proto")
	if err := os.MkdirAll(proto, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(proto, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(api, "genguard.yaml"),
		filepath.Join(proto, "genguard.yaml"),
	}
	assertPaths(t, found, want)
}

func TestFindAllSkipsGitVendorNodeModules(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	hidden := []string{
		filepath.Join(root, ".git", "hooks"),
		filepath.Join(root, "vendor", "lib"),
		filepath.Join(root, "web", "node_modules", "pkg"),
	}
	for _, dir := range hidden {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := testutil.WriteGenguardConfig(dir, "gen/", "", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	api := filepath.Join(root, "api")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(api, "genguard.yaml")})
}

func TestFindAllEmpty(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %v, want empty", found)
	}
}

func TestFindAllDoesNotRequireGit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}

	_, err := listConfigs(t, root)
	if err == nil || !strings.Contains(err.Error(), "git") {
		t.Fatalf("error = %v", err)
	}
}

func TestFindAllRejectsBothNamesInSameDir(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	web := filepath.Join(root, "web")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "genguard.yaml", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "genguard.yml", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(web, "gen/", "", "genguard.yaml", nil); err != nil {
		t.Fatal(err)
	}

	_, err := listConfigs(t, root)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "both genguard.yaml and genguard.yml") || !strings.Contains(err.Error(), api) {
		t.Fatalf("error = %q", err)
	}
}

func TestFindAllRejectsBothNamesWhenNestedPathSortsBetween(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	// genguard.yaml.bak sorts between the two config names.
	backup := filepath.Join(api, "genguard.yaml.bak")
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "genguard.yaml", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "genguard.yml", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(backup, "gen/", "", "genguard.yaml", nil); err != nil {
		t.Fatal(err)
	}

	_, err := listConfigs(t, root)
	if err == nil || err.Error() != api+" contains both genguard.yaml and genguard.yml; keep one" {
		t.Fatalf("error = %v", err)
	}
}

func TestFindAllAllowsConfigDirectoryName(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "genguard.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "genguard.yml", nil); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(root, "genguard.yml")})
}

func TestFindAllFollowsSymlinkRoot(t *testing.T) {
	t.Parallel()
	realRoot := initRepo(t)
	if _, err := testutil.WriteGenguardConfig(realRoot, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, link)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(resolved, "genguard.yaml")})
}

func TestFindAllReturnsAbsolutePaths(t *testing.T) {
	root := initRepo(t)
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}

	// filepath.Rel cannot cross Windows drive letters.
	testutil.Chdir(t, filepath.Dir(root))
	found, err := listConfigs(t, filepath.Base(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("found = %v, want 1 path", found)
	}
	if !filepath.IsAbs(found[0]) {
		t.Fatalf("path is not absolute: %q", found[0])
	}
	got, err := filepath.EvalSymlinks(found[0])
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("found = %q, want %q", found[0], filepath.Join(root, "genguard.yaml"))
	}
}

func TestFindAllIgnoresDirectoryNamedLikeConfig(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "genguard.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %v, want empty", found)
	}
}

func TestFindAllMissingRoot(t *testing.T) {
	t.Parallel()
	_, err := listConfigs(t, filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindAllRejectsFileRoot(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	path := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := listConfigs(t, path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %q", err)
	}
}

func TestFindAllDanglingSymlinkRoot(t *testing.T) {
	t.Parallel()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), link); err != nil {
		t.Fatal(err)
	}
	_, err := listConfigs(t, link)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDiscoveryWhenWorkingDirectoryIsGone(t *testing.T) {
	testutil.WithoutWorkingDirectory(t)

	if _, err := listConfigs(t, ""); err == nil {
		t.Fatal("FindAll empty start")
	}
	if _, err := listConfigs(t, "repo"); err == nil {
		t.Fatal("FindAll relative start")
	}
	if _, err := config.FindConfig("", ""); err == nil {
		t.Fatal("FindConfig empty start")
	}
	if _, err := config.FindConfig("repo", ""); err == nil {
		t.Fatal("FindConfig relative start")
	}
}

func TestFindAllEmptyStartUsesCwd(t *testing.T) {
	root := initRepo(t)
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, root)

	found, err := listConfigs(t, "")
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(root, "genguard.yaml")})
}

func TestFindAllOmitsGitignoredConfig(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	hidden := filepath.Join(root, "hidden")
	api := filepath.Join(root, "api")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("hidden/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(hidden, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", ".gitignore", "api/genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "api"); err != nil {
		t.Fatal(err)
	}

	want := []string{filepath.Join(api, "genguard.yaml")}
	for _, isolated := range []bool{false, true} {
		found, err := discover.FindAll(context.Background(), root, isolated)
		if err != nil {
			t.Fatal(err)
		}
		assertPaths(t, found, want)
	}
}

func TestFindAllUntrackedConfigIsWorkingTreeOnly(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	web := filepath.Join(root, "web")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(api, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(web, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "web/genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "web"); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{
		filepath.Join(api, "genguard.yaml"),
		filepath.Join(web, "genguard.yaml"),
	})
	indexed, err := discover.FindAll(context.Background(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, indexed, []string{filepath.Join(web, "genguard.yaml")})
}

func TestFindAllOmitsCommittedSkipDirs(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	vendor := filepath.Join(root, "vendor", "lib")
	modules := filepath.Join(root, "web", "node_modules", "pkg")
	for _, dir := range []string{api, vendor, modules} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := testutil.WriteGenguardConfig(dir, "gen/", "", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(root, "add", "-f", "api/genguard.yaml", "vendor/lib/genguard.yaml", "web/node_modules/pkg/genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "configs"); err != nil {
		t.Fatal(err)
	}

	want := []string{filepath.Join(api, "genguard.yaml")}
	for _, isolated := range []bool{false, true} {
		found, err := discover.FindAll(context.Background(), root, isolated)
		if err != nil {
			t.Fatal(err)
		}
		assertPaths(t, found, want)
	}
}

func TestFindAllIsolatedListsHeadNotTheIndex(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	web := filepath.Join(root, "web")
	extra := filepath.Join(root, "extra")
	for _, dir := range []string{api, web, extra} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := testutil.WriteGenguardConfig(dir, "gen/", "", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(root, "add", "api/genguard.yaml", "web/genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "configs"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "rm", "api/genguard.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "extra/genguard.yaml"); err != nil {
		t.Fatal(err)
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{
		filepath.Join(extra, "genguard.yaml"),
		filepath.Join(web, "genguard.yaml"),
	})
	head, err := discover.FindAll(context.Background(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, head, []string{
		filepath.Join(api, "genguard.yaml"),
		filepath.Join(web, "genguard.yaml"),
	})
}

func TestFindAllUnreadableDirectoryFails(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	nested := filepath.Join(root, "blocked", "nested")
	for _, dir := range []string{api, nested} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := testutil.WriteGenguardConfig(dir, "gen/", "", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	blocked := filepath.Join(root, "blocked")
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	if f, err := os.Open(blocked); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced")
	}

	_, err := listConfigs(t, root)
	if err == nil || !strings.Contains(err.Error(), "could not open directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestFindAllUnreadableSkipDirStillLists(t *testing.T) {
	t.Parallel()
	root := initRepo(t)
	api := filepath.Join(root, "api")
	vendor := filepath.Join(root, "vendor", "lib")
	for _, dir := range []string{api, vendor} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := testutil.WriteGenguardConfig(dir, "gen/", "", "", nil); err != nil {
			t.Fatal(err)
		}
	}
	vendorRoot := filepath.Join(root, "vendor")
	if err := os.Chmod(vendorRoot, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(vendorRoot, 0o755) })
	if f, err := os.Open(vendorRoot); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced")
	}

	found, err := listConfigs(t, root)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(api, "genguard.yaml")})
}

func initRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func listConfigs(t *testing.T, root string) ([]string, error) {
	t.Helper()
	return discover.FindAll(context.Background(), root, false)
}

func assertPaths(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("found %d paths %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("found[%d] = %q, want %q\nfull: %v", i, got[i], want[i], got)
		}
	}
}
