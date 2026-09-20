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

	err := removeCleanTarget(generated, true)
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
