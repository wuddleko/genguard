package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestFindAllNestedYamlAndYml(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
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

	found, err := config.FindAll(root)
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
	root := t.TempDir()
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

	found, err := config.FindAll(root)
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
	root := t.TempDir()
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

	found, err := config.FindAll(root)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(api, "genguard.yaml")})
}

func TestFindAllEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := config.FindAll(root)
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

	found, err := config.FindAll(root)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(root, "genguard.yaml")})
}

func TestFindAllRejectsBothNamesInSameDir(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
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

	_, err := config.FindAll(root)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "both genguard.yaml and genguard.yml") || !strings.Contains(err.Error(), api) {
		t.Fatalf("error = %q", err)
	}
}

func TestFindAllRejectsBothNamesWhenNestedPathSortsBetween(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	api := filepath.Join(root, "api")
	// genguard.yaml.bak sorts between genguard.yaml and genguard.yml.
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

	_, err := config.FindAll(root)
	if err == nil || err.Error() != api+" contains both genguard.yaml and genguard.yml; keep one" {
		t.Fatalf("error = %v", err)
	}
}

func TestFindAllAllowsConfigDirectoryName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "genguard.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "genguard.yml", nil); err != nil {
		t.Fatal(err)
	}

	found, err := config.FindAll(root)
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(root, "genguard.yml")})
}

func TestFindAllFollowsSymlinkRoot(t *testing.T) {
	t.Parallel()
	realRoot := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(realRoot, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "repo")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}

	found, err := config.FindAll(link)
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
	t.Parallel()
	root := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}

	rel, err := filepath.Rel(mustGetwd(t), root)
	if err != nil {
		t.Fatal(err)
	}
	found, err := config.FindAll(rel)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("found = %v, want 1 path", found)
	}
	if !filepath.IsAbs(found[0]) {
		t.Fatalf("path is not absolute: %q", found[0])
	}
	if found[0] != filepath.Join(root, "genguard.yaml") {
		t.Fatalf("found = %q", found[0])
	}
}

func TestFindAllIgnoresDirectoryNamedLikeConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "genguard.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	found, err := config.FindAll(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("found = %v, want empty", found)
	}
}

func TestFindAllMissingRoot(t *testing.T) {
	t.Parallel()
	_, err := config.FindAll(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindAllRejectsFileRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := config.FindAll(path)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %q", err)
	}
}

func TestFindAllEmptyStartUsesCwd(t *testing.T) {
	root := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, root)

	found, err := config.FindAll("")
	if err != nil {
		t.Fatal(err)
	}
	assertPaths(t, found, []string{filepath.Join(root, "genguard.yaml")})
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

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
