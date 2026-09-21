package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
)

func TestResolveCleanTargetDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target, isDir, err := resolveCleanTarget(root, "generated/")
	if err != nil {
		t.Fatal(err)
	}
	if !isDir {
		t.Fatal("want directory")
	}
	if filepath.Base(target) != "generated" {
		t.Fatalf("target = %q", target)
	}
}

func TestResolveCleanTargetRefuses(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cases := []struct {
		spec  string
		match string
	}{
		{".", `clean refuses "."`},
		{"./", `clean refuses "./"`},
		{"..", `clean refuses ".."`},
		{"../", `clean refuses "../"`},
		{"foo/../..", "clean refuses"},
		{"generated/..", "clean refuses"},
		{"generated/*.txt", "glob"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			_, _, err := resolveCleanTarget(root, tc.spec)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.match) {
				t.Fatalf("error = %q, want substring %q", err, tc.match)
			}
		})
	}
}

func TestResolveCleanTargetRefusesAbsolute(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	spec := filepath.Join(root, "generated")
	_, _, err := resolveCleanTarget(root, spec)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error = %q", err)
	}
}

func TestResolveCleanTargetRefusesSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "generated")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	_, _, err := resolveCleanTarget(root, "generated")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %q", err)
	}
}

func TestResolveCleanTargetRefusesIntermediateSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "sub", "foo.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "generated")); err != nil {
		t.Fatal(err)
	}

	_, _, err := resolveCleanTarget(root, "generated/sub/foo.txt")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "symlink in output path") {
		t.Fatalf("error = %q", err)
	}
}

func TestResolveCleanTargetRefusesNestedSymlinkComponent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(generated, "out")); err != nil {
		t.Fatal(err)
	}

	_, _, err := resolveCleanTarget(root, "generated/out/")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %q", err)
	}
}

func TestCleanOutputsPreservesExternalSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	outside := filepath.Join(root, "outside")
	secret := filepath.Join(outside, "secret.txt")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "hello.txt"), []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(generated, "link")); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, config.Group{Outputs: []string{"generated/"}}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(secret)
	if err != nil {
		t.Fatalf("outside secret missing: %v", err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("secret = %q", got)
	}
	if _, err := os.Stat(filepath.Join(generated, "link")); !os.IsNotExist(err) {
		t.Fatalf("symlink entry should be removed: %v", err)
	}
}

func TestCleanOutputsIgnoresGitBehindSymlink(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	outside := filepath.Join(root, "outside")
	outsideGit := filepath.Join(outside, ".git")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideGit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideGit, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "hello.txt"), []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(generated, "link")); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, config.Group{Outputs: []string{"generated/"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(outsideGit, "HEAD")); err != nil {
		t.Fatalf("git behind symlink was removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(generated, "link")); !os.IsNotExist(err) {
		t.Fatalf("symlink entry should be removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(generated, "hello.txt")); !os.IsNotExist(err) {
		t.Fatalf("hello.txt should be removed: %v", err)
	}
}

func TestCleanOutputsGitNameUsesFilesystemCase(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	nested := filepath.Join(generated, "sub", ".GIT")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "hello.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, foldErr := os.Lstat(filepath.Join(generated, "sub", ".git"))
	err := cleanOutputs(root, config.Group{Outputs: []string{"generated/"}})
	if os.IsNotExist(foldErr) {
		if err != nil {
			t.Fatal(err)
		}
		if _, statErr := os.Lstat(filepath.Join(generated, "hello.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("hello.txt should be removed: %v", statErr)
		}
		if _, statErr := os.Lstat(nested); !os.IsNotExist(statErr) {
			t.Fatalf(".GIT should be removed: %v", statErr)
		}
		return
	}
	if foldErr != nil {
		t.Fatal(foldErr)
	}
	if err == nil || !strings.Contains(err.Error(), `mixed tree (would delete generated/sub/.git)`) {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(nested, "HEAD")); statErr != nil {
		t.Fatalf("nested git removed: %v", statErr)
	}
	if _, statErr := os.Lstat(filepath.Join(generated, "hello.txt")); statErr != nil {
		t.Fatalf("generated file removed: %v", statErr)
	}
}

func TestCleanOutputsRefusesConfigFile(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	if err := os.WriteFile(configPath, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, config.Group{Outputs: []string{"genguard.yaml"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %q", err)
	}
}

func TestCleanOutputsRefusesGitDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, config.Group{Outputs: []string{".git/"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), ".git") {
		t.Fatalf("error = %q", err)
	}
}

func TestRemoveCleanTargetRefusesSwappedSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	secret := filepath.Join(outside, "secret.txt")
	generated := filepath.Join(root, "generated")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(generated, "hello.txt"), []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(generated); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, generated); err != nil {
		t.Fatal(err)
	}

	err := removeCleanTarget(root, generated, true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %q", err)
	}
	got, readErr := os.ReadFile(secret)
	if readErr != nil {
		t.Fatalf("outside secret missing: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("secret = %q", got)
	}
}

func TestCleanOutputsRefusesDirectoryReplacedWithSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	secret := filepath.Join(outside, "secret.txt")
	generated := filepath.Join(root, "generated")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(generated); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, generated); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, config.Group{Outputs: []string{"generated/"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %q", err)
	}
	got, readErr := os.ReadFile(secret)
	if readErr != nil {
		t.Fatalf("outside secret missing: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("secret = %q", got)
	}
}

func TestRefuseSymlinksInPathRefusesDotDot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := refuseSymlinksInPath(root, filepath.Join(root, "..", "outside"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "clean refuses") {
		t.Fatalf("error = %q", err)
	}
}

func TestRemoveCleanTargetRemovesNestedFile(t *testing.T) {
	root := t.TempDir()
	generated := filepath.Join(root, "generated")
	hello := filepath.Join(generated, "hello.txt")
	if err := os.Mkdir(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := removeCleanTarget(root, hello, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(hello); !os.IsNotExist(err) {
		t.Fatalf("file should be removed: %v", err)
	}
	if _, err := os.Lstat(generated); err != nil {
		t.Fatalf("parent directory should remain: %v", err)
	}
}

func TestRemoveCleanTargetRefusesParentSymlinkFile(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	secret := filepath.Join(outside, "hello.txt")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "generated")); err != nil {
		t.Fatal(err)
	}

	err := removeCleanTarget(root, filepath.Join(root, "generated", "hello.txt"), false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %q", err)
	}
	got, readErr := os.ReadFile(secret)
	if readErr != nil {
		t.Fatalf("outside file missing: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("secret = %q", got)
	}
}

func TestRemoveCleanTargetRefusesParentSymlinkMkdir(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "generated")); err != nil {
		t.Fatal(err)
	}

	err := removeCleanTarget(root, filepath.Join(root, "generated", "nested"), true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %q", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "nested")); !os.IsNotExist(err) {
		t.Fatalf("should not create through symlink: %v", err)
	}
}

func TestRemoveCleanTargetCreatesMissingDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "generated", "nested")
	if err := removeCleanTarget(root, target, true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("want directory")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("want a real directory")
	}
}
