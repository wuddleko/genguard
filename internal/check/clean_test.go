package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Error(err)
		}
	})
}

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

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/"}}); err != nil {
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

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/"}}); err != nil {
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
	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/"}})
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

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"genguard.yaml"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %q", err)
	}
}

func TestCleanOutputsRefusesLoadedConfig(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "custom.yaml")
	if err := os.WriteFile(configPath, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, configPath, config.Group{Outputs: []string{"custom.yaml"}})
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(configPath); statErr != nil {
		t.Fatalf("config removed: %v", statErr)
	}
}

func TestCleanOutputsRefusesLoadedConfigOtherCase(t *testing.T) {
	root := t.TempDir()
	loaded := filepath.Join(root, "custom.yaml")
	other := filepath.Join(root, "Custom.yaml")
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loaded, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, foldErr := os.Lstat(other)
	err := cleanOutputs(root, loaded, config.Group{Outputs: []string{"generated/hello.txt", "Custom.yaml"}})
	if os.IsNotExist(foldErr) {
		if err != nil {
			t.Fatal(err)
		}
		if _, statErr := os.Lstat(hello); !os.IsNotExist(statErr) {
			t.Fatalf("hello.txt = %v", statErr)
		}
		if _, statErr := os.Lstat(loaded); statErr != nil {
			t.Fatalf("config removed: %v", statErr)
		}
		return
	}
	if foldErr != nil {
		t.Fatal(foldErr)
	}
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(hello); statErr != nil {
		t.Fatalf("hello.txt removed: %v", statErr)
	}
	got, readErr := os.ReadFile(loaded)
	if readErr != nil {
		t.Fatalf("config removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("config = %q", got)
	}
}

func TestCleanOutputsGlobRefusesLoadedConfig(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	configPath := filepath.Join(root, "custom.yaml")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, configPath, config.Group{Outputs: []string{"generated/hello.txt", "*.yaml"}})
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(hello)
	if readErr != nil {
		t.Fatalf("hello.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("hello.txt = %q", got)
	}
	if _, statErr := os.Lstat(configPath); statErr != nil {
		t.Fatalf("config removed: %v", statErr)
	}
}

func TestCleanOutputsGlobRefusesLoadedConfigOtherCase(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	loaded := filepath.Join(root, "custom.yaml")
	other := filepath.Join(root, "Custom.yaml")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loaded, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	_, foldErr := os.Lstat(other)
	err := cleanOutputs(root, other, config.Group{Outputs: []string{"generated/hello.txt", "*.yaml"}})
	if os.IsNotExist(foldErr) {
		if err != nil {
			t.Fatal(err)
		}
		if _, statErr := os.Lstat(hello); !os.IsNotExist(statErr) {
			t.Fatalf("hello.txt = %v", statErr)
		}
		if _, statErr := os.Lstat(loaded); !os.IsNotExist(statErr) {
			t.Fatalf("custom.yaml = %v", statErr)
		}
		return
	}
	if foldErr != nil {
		t.Fatal(foldErr)
	}
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(hello)
	if readErr != nil {
		t.Fatalf("hello.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("hello.txt = %q", got)
	}
	got, readErr = os.ReadFile(loaded)
	if readErr != nil {
		t.Fatalf("config removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("config = %q", got)
	}
}

func TestCleanOutputsRefusesLoadedConfigSymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real.yaml")
	link := filepath.Join(root, "custom.yaml")
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.yaml", link); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, link, config.Group{Outputs: []string{"generated/hello.txt", "real.yaml"}})
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(real)
	if readErr != nil {
		t.Fatalf("target removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("target = %q", got)
	}
	if _, statErr := os.Lstat(link); statErr != nil {
		t.Fatalf("link removed: %v", statErr)
	}
	got, readErr = os.ReadFile(hello)
	if readErr != nil {
		t.Fatalf("hello.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("hello.txt = %q", got)
	}
}

func TestCleanOutputsRefusesLoadedConfigSymlinkInDirectory(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "generated", "sub", "real.yaml")
	link := filepath.Join(root, "custom.yaml")
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("generated", "sub", "real.yaml"), link); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, link, config.Group{Outputs: []string{"generated/"}})
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(link)
	if readErr != nil {
		t.Fatalf("target removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("target = %q", got)
	}
	if _, statErr := os.Lstat(hello); statErr != nil {
		t.Fatalf("hello.txt removed: %v", statErr)
	}
}

func TestCleanOutputsRefusesLoadedConfigSymlinkInOtherCaseDirectory(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "generated", "real.yaml")
	link := filepath.Join(root, "custom.yaml")
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("generated", "real.yaml"), link); err != nil {
		t.Fatal(err)
	}

	_, foldErr := os.Lstat(filepath.Join(root, "Generated"))
	err := cleanOutputs(root, link, config.Group{Outputs: []string{"Generated/"}})
	if os.IsNotExist(foldErr) {
		if err != nil {
			t.Fatal(err)
		}
		if _, statErr := os.Lstat(real); statErr != nil {
			t.Fatalf("target removed: %v", statErr)
		}
		if _, statErr := os.Lstat(hello); statErr != nil {
			t.Fatalf("hello.txt removed: %v", statErr)
		}
		return
	}
	if foldErr != nil {
		t.Fatal(foldErr)
	}
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(link)
	if readErr != nil {
		t.Fatalf("target removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("target = %q", got)
	}
	if _, statErr := os.Lstat(hello); statErr != nil {
		t.Fatalf("hello.txt removed: %v", statErr)
	}
}

func TestCleanOutputsRefusesStandardConfigSymlinkInDirectory(t *testing.T) {
	for _, name := range []string{"genguard.yaml", "genguard.yml"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			real := filepath.Join(root, "generated", "kept.yaml")
			link := filepath.Join(root, name)
			hello := filepath.Join(root, "generated", "hello.txt")
			if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(real, []byte("keep\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join("generated", "kept.yaml"), link); err != nil {
				t.Fatal(err)
			}

			err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/"}})
			if err == nil || !strings.Contains(err.Error(), "config file") {
				t.Fatalf("error = %v", err)
			}
			got, readErr := os.ReadFile(link)
			if readErr != nil {
				t.Fatalf("target removed: %v", readErr)
			}
			if string(got) != "keep\n" {
				t.Fatalf("target = %q", got)
			}
			if _, statErr := os.Lstat(hello); statErr != nil {
				t.Fatalf("hello.txt removed: %v", statErr)
			}
		})
	}
}

func TestCleanOutputsRemovesDirectoryWhenConfigSymlinkIsOutside(t *testing.T) {
	root := t.TempDir()
	kept := filepath.Join(root, "kept", "real.yaml")
	link := filepath.Join(root, "custom.yaml")
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("kept", "real.yaml"), link); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, link, config.Group{Outputs: []string{"generated/"}}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(link)
	if err != nil {
		t.Fatalf("target removed: %v", err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("target = %q", got)
	}
	if _, statErr := os.Lstat(hello); !os.IsNotExist(statErr) {
		t.Fatalf("hello.txt = %v", statErr)
	}
}

func TestCleanOutputsRefusesStandardConfigOtherCase(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "genguard.yaml")
	if err := os.WriteFile(path, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, foldErr := os.Lstat(filepath.Join(root, "Genguard.yaml"))
	err := cleanOutputs(root, "", config.Group{Outputs: []string{"Genguard.yaml"}})
	if os.IsNotExist(foldErr) {
		if err != nil {
			t.Fatal(err)
		}
		if _, statErr := os.Lstat(path); statErr != nil {
			t.Fatalf("config removed: %v", statErr)
		}
		return
	}
	if foldErr != nil {
		t.Fatal(foldErr)
	}
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(path); statErr != nil {
		t.Fatalf("config removed: %v", statErr)
	}
}

func TestCleanOutputsStillRefusesStandardConfigNames(t *testing.T) {
	root := t.TempDir()
	loaded := filepath.Join(root, "custom.yaml")
	if err := os.WriteFile(loaded, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"genguard.yaml", "genguard.yml"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("groups: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := cleanOutputs(root, loaded, config.Group{Outputs: []string{name}})
		if err == nil || !strings.Contains(err.Error(), "config file") {
			t.Fatalf("%s error = %v", name, err)
		}
		if _, statErr := os.Lstat(path); statErr != nil {
			t.Fatalf("%s removed: %v", name, statErr)
		}
	}
}

func TestCleanOutputsRemovesOtherFilesWhenConfigIsProtected(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "custom.yaml")
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, configPath, config.Group{Outputs: []string{"generated/hello.txt"}}); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Lstat(hello); !os.IsNotExist(statErr) {
		t.Fatalf("hello.txt = %v", statErr)
	}
	if _, statErr := os.Lstat(configPath); statErr != nil {
		t.Fatalf("config removed: %v", statErr)
	}
}

func TestCleanOutputsRefusesNestedConfig(t *testing.T) {
	for _, name := range []string{"genguard.yaml", "genguard.yml"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			loaded := filepath.Join(root, "custom.yaml")
			nested := filepath.Join(root, "services", "api", name)
			hello := filepath.Join(root, "services", "hello.txt")
			if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(loaded, []byte("keep\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(nested, []byte("keep\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			err := cleanOutputs(root, loaded, config.Group{Outputs: []string{"services/"}})
			if err == nil || !strings.Contains(err.Error(), "config file") {
				t.Fatalf("error = %v", err)
			}
			if _, statErr := os.Lstat(hello); statErr != nil {
				t.Fatalf("hello.txt removed: %v", statErr)
			}
			got, readErr := os.ReadFile(nested)
			if readErr != nil || string(got) != "keep\n" {
				t.Fatalf("nested = %q, %v", got, readErr)
			}

			err = cleanOutputs(root, loaded, config.Group{Outputs: []string{filepath.Join("services", "api", name)}})
			if err == nil || !strings.Contains(err.Error(), "config file") {
				t.Fatalf("file error = %v", err)
			}
			if _, statErr := os.Lstat(nested); statErr != nil {
				t.Fatalf("nested removed: %v", statErr)
			}
		})
	}
}

func TestCleanOutputsRemovesExtraHardLinkOfConfig(t *testing.T) {
	root := t.TempDir()
	loaded := filepath.Join(root, "custom.yaml")
	extra := filepath.Join(root, "generated", "copy.yaml")
	backup := filepath.Join(root, "backup.yaml")
	if err := os.MkdirAll(filepath.Dir(extra), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loaded, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(loaded, extra); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(loaded, backup); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, loaded, config.Group{Outputs: []string{"custom.yaml"}})
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(loaded)
	if readErr != nil || string(got) != "keep\n" {
		t.Fatalf("config = %q, %v", got, readErr)
	}
	if _, statErr := os.Lstat(backup); statErr != nil {
		t.Fatalf("backup removed: %v", statErr)
	}

	if err := cleanOutputs(root, loaded, config.Group{Outputs: []string{"generated/copy.yaml"}}); err != nil {
		t.Fatal(err)
	}
	got, readErr = os.ReadFile(loaded)
	if readErr != nil || string(got) != "keep\n" {
		t.Fatalf("config after extra link = %q, %v", got, readErr)
	}
	if _, statErr := os.Lstat(extra); !os.IsNotExist(statErr) {
		t.Fatalf("copy.yaml = %v", statErr)
	}
}

func TestCleanOutputsRefusesUnresolvedLoadedConfig(t *testing.T) {
	root := t.TempDir()
	linkA := filepath.Join(root, "a.yaml")
	linkB := filepath.Join(root, "b.yaml")
	hello := filepath.Join(root, "hello.txt")
	if err := os.Symlink("b.yaml", linkA); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.yaml", linkB); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, linkA, config.Group{Outputs: []string{"hello.txt"}})
	if err == nil {
		t.Fatal("expected error")
	}
	got, readErr := os.ReadFile(hello)
	if readErr != nil || string(got) != "keep\n" {
		t.Fatalf("hello.txt = %q, %v", got, readErr)
	}
}

func TestCheckAndRunCleanRefuseLoadedConfig(t *testing.T) {
	for _, relative := range []bool{false, true} {
		for _, run := range []bool{false, true} {
			name := "check"
			if run {
				name = "run"
			}
			if relative {
				name += "-relative"
			}
			t.Run(name, func(t *testing.T) {
				parent := t.TempDir()
				root := filepath.Join(parent, "repo")
				if err := testutil.InitGitRepo(root); err != nil {
					t.Fatal(err)
				}
				path, err := testutil.WriteGenguardConfig(root, "", "", "custom.yaml", []testutil.GroupSpec{{
					Name:    "gen",
					Command: `python3 -c "open('ran','w').close()"`,
					Outputs: []string{"custom.yaml"},
					Clean:   true,
				}})
				if err != nil {
					t.Fatal(err)
				}
				loadPath := path
				if relative {
					chdir(t, parent)
					loadPath = filepath.Join("repo", "custom.yaml")
				}
				cfg, err := config.LoadConfig(loadPath)
				if err != nil {
					t.Fatal(err)
				}
				if !filepath.IsAbs(cfg.Path) {
					t.Fatalf("config path = %q", cfg.Path)
				}
				loadedInfo, err := os.Stat(cfg.Path)
				if err != nil {
					t.Fatal(err)
				}
				wantInfo, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(loadedInfo, wantInfo) {
					t.Fatalf("config path = %q", cfg.Path)
				}
				var result ConfigResult
				if run {
					result, err = RunConfig(cfg)
				} else {
					result, err = CheckConfig(cfg)
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Groups) != 1 || result.Groups[0].Status != GroupError || result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "config file") {
					t.Fatalf("result = %+v", result.Groups)
				}
				if _, statErr := os.Lstat(path); statErr != nil {
					t.Fatalf("config removed: %v", statErr)
				}
				if _, statErr := os.Lstat(filepath.Join(root, "ran")); !os.IsNotExist(statErr) {
					t.Fatalf("command ran: %v", statErr)
				}
			})
		}
	}
}

func TestCleanOutputsRefusesGitDir(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{".git/"}})
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

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/"}})
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

func TestCleanOutputsRefusalDeletesNothing(t *testing.T) {
	root := t.TempDir()
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/hello.txt", ".."}})
	if err == nil || !strings.Contains(err.Error(), `clean refuses ".."`) {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(hello)
	if readErr != nil {
		t.Fatalf("hello.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("hello.txt = %q", got)
	}
}

func TestCleanOutputsGlobDeletesMatchesOnly(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(root, "generated")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(generated, "hello.txt")
	keep := filepath.Join(generated, "keep.go")
	ignored := filepath.Join(generated, "ignored.txt")
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("generated/ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(generated, "extra.txt")
	if err := os.WriteFile(extra, []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignored, []byte("ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/*.txt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(hello); !os.IsNotExist(err) {
		t.Fatalf("hello.txt should be removed: %v", err)
	}
	if _, err := os.Lstat(extra); !os.IsNotExist(err) {
		t.Fatalf("extra.txt should be removed: %v", err)
	}
	got, err := os.ReadFile(keep)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hand\n" {
		t.Fatalf("keep.go = %q", got)
	}
	got, err = os.ReadFile(ignored)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ignored\n" {
		t.Fatalf("ignored.txt = %q", got)
	}
}

func TestCleanOutputsGlobFromSubdirectory(t *testing.T) {
	repo := t.TempDir()
	if err := testutil.InitGitRepo(repo); err != nil {
		t.Fatal(err)
	}
	api := filepath.Join(repo, "api")
	hello := filepath.Join(api, "generated", "hello.txt")
	keep := filepath.Join(api, "generated", "keep.go")
	secret := filepath.Join(repo, "secret.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(repo, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(repo, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(api, "", config.Group{Outputs: []string{"generated/*.txt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(hello); !os.IsNotExist(err) {
		t.Fatalf("hello.txt should be removed: %v", err)
	}
	got, err := os.ReadFile(keep)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hand\n" {
		t.Fatalf("keep.go = %q", got)
	}
	got, err = os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret\n" {
		t.Fatalf("secret.txt = %q", got)
	}
}

func TestCleanOutputsGlobRefusesConfigWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	configPath := filepath.Join(root, "genguard.yaml")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/hello.txt", "*.yaml"}})
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(hello)
	if readErr != nil {
		t.Fatalf("hello.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("hello.txt = %q", got)
	}
	if _, statErr := os.Lstat(configPath); statErr != nil {
		t.Fatalf("config removed: %v", statErr)
	}
}

func TestCleanOutputsGlobRefusesSymlinkWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(root, "generated")
	hello := filepath.Join(generated, "hello.txt")
	keep := filepath.Join(root, "notes", "keep.txt")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(root, "outside")
	secret := filepath.Join(outside, "secret.txt")
	outsideHello := filepath.Join(outside, "hello.txt")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideHello, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(generated); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, generated); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"notes/keep.txt", "generated/*.txt"}})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(keep)
	if readErr != nil {
		t.Fatalf("keep.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("keep.txt = %q", got)
	}
	got, readErr = os.ReadFile(secret)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "secret\n" {
		t.Fatalf("secret.txt = %q", got)
	}
	got, readErr = os.ReadFile(outsideHello)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "outside\n" {
		t.Fatalf("outside hello.txt = %q", got)
	}
}

func TestCleanOutputsGlobRefusesParentEscape(t *testing.T) {
	root := t.TempDir()
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/hello.txt", "../*"}})
	if err == nil || !strings.Contains(err.Error(), `clean refuses "../*"`) {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(hello); statErr != nil {
		t.Fatalf("hello.txt removed: %v", statErr)
	}
}

func TestCleanOutputsGlobSqlcPackage(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "db.go")
	models := filepath.Join(root, "models.go")
	oidc := filepath.Join(root, "oidc_queries.sql.go")
	session := filepath.Join(root, "session_queries.sql.go")
	skipped := filepath.Join(root, "skip_queries.sql.go")
	if err := os.WriteFile(db, []byte("package db\n\nfunc Queries() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(models, []byte("package db\n\ntype OidcSession struct{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oidc, []byte("package db\n\nfunc Oidc() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("skip_queries.sql.go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte("package db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skipped, []byte("package skip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"*_queries.sql.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(oidc); !os.IsNotExist(err) {
		t.Fatalf("oidc_queries.sql.go should be removed: %v", err)
	}
	if _, err := os.Lstat(session); !os.IsNotExist(err) {
		t.Fatalf("untracked session_queries.sql.go should be removed: %v", err)
	}
	for _, path := range []string{db, models, skipped} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("%s removed: %v", filepath.Base(path), err)
		}
	}
}

func TestCleanOutputsGlobSlashFreeStaysInConfigDirectory(t *testing.T) {
	repo := t.TempDir()
	if err := testutil.InitGitRepo(repo); err != nil {
		t.Fatal(err)
	}
	sqlite := filepath.Join(repo, "sqlite")
	other := filepath.Join(repo, "other")
	if err := os.MkdirAll(sqlite, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(sqlite, "db.go")
	oidc := filepath.Join(sqlite, "oidc_queries.sql.go")
	sibling := filepath.Join(other, "bar_queries.sql.go")
	nested := filepath.Join(sqlite, "store", "nested_queries.sql.go")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{db, oidc, sibling, nested} {
		if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(repo, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(repo, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(sqlite, "", config.Group{Outputs: []string{"*_queries.sql.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(oidc); !os.IsNotExist(err) {
		t.Fatalf("sqlite query file should be removed: %v", err)
	}
	if _, err := os.Lstat(nested); !os.IsNotExist(err) {
		t.Fatalf("nested query file under the config directory should be removed: %v", err)
	}
	if _, err := os.Lstat(db); err != nil {
		t.Fatalf("db.go removed: %v", err)
	}
	got, err := os.ReadFile(sibling)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "package x\n" {
		t.Fatalf("sibling = %q", got)
	}
}

func TestCleanOutputsGlobSlashFreeFromRepoRootMatchesEveryPackage(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	sqlite := filepath.Join(root, "sqlite", "oidc_queries.sql.go")
	other := filepath.Join(root, "other", "bar_queries.sql.go")
	hand := filepath.Join(root, "sqlite", "db.go")
	if err := os.MkdirAll(filepath.Dir(sqlite), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{sqlite, other, hand} {
		if err := os.WriteFile(path, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"*_queries.sql.go"}}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{sqlite, other} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed: %v", path, err)
		}
	}
	if _, err := os.Lstat(hand); err != nil {
		t.Fatalf("db.go removed: %v", err)
	}
}

func TestCleanOutputsGlobQuestionMarkAndClass(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(root, "generated")
	if err := os.MkdirAll(generated, 0o755); err != nil {
		t.Fatal(err)
	}
	one := filepath.Join(generated, "a1.txt")
	word := filepath.Join(generated, "ab.txt")
	longer := filepath.Join(generated, "abc.txt")
	keep := filepath.Join(generated, "keep.go")
	for _, path := range []string{one, word, longer, keep} {
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/a?.txt"}}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{one, word} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed: %v", filepath.Base(path), err)
		}
	}
	for _, path := range []string{longer, keep} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("%s removed: %v", filepath.Base(path), err)
		}
	}

	if err := os.WriteFile(one, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/a[0-9].txt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(one); !os.IsNotExist(err) {
		t.Fatalf("a1.txt should be removed: %v", err)
	}
	if _, err := os.Lstat(longer); err != nil {
		t.Fatalf("abc.txt removed: %v", err)
	}
}

func TestCleanOutputsGlobStarStar(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	top := filepath.Join(root, "generated", "a.txt")
	nested := filepath.Join(root, "generated", "sub", "b.txt")
	keep := filepath.Join(root, "generated", "sub", "keep.go")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{top, nested, keep} {
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/**.txt"}}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{top, nested} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed: %v", path, err)
		}
	}
	if _, err := os.Lstat(keep); err != nil {
		t.Fatalf("keep.go removed: %v", err)
	}
}

func TestCleanOutputsGlobMatchesNothing(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "db.go")
	if err := os.WriteFile(keep, []byte("package db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"missing/*.txt", "*_queries.sql.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(keep); err != nil {
		t.Fatalf("db.go removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing directory created: %v", err)
	}
}

func TestCleanOutputsGlobTrimsPattern(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"  generated/*.txt  "}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(hello); !os.IsNotExist(err) {
		t.Fatalf("hello.txt should be removed: %v", err)
	}
}

func TestCleanOutputsGlobDeletesAlreadyMissingTrackedFile(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	keep := filepath.Join(root, "generated", "keep.go")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(hello); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/*.txt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(hello); !os.IsNotExist(err) {
		t.Fatalf("hello.txt came back: %v", err)
	}
	got, err := os.ReadFile(keep)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hand\n" {
		t.Fatalf("keep.go = %q", got)
	}
}

func TestCleanOutputsGlobNewlineInFilename(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	rel := "generated/hello\nworld.txt"
	hello := filepath.Join(root, rel)
	keep := filepath.Join(root, "generated", "keep.go")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.SkipIfFilenameRejected(t, filepath.Dir(hello), "hello\nworld.txt")
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/*.txt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(hello); !os.IsNotExist(err) {
		t.Fatalf("newline file should be removed: %v", err)
	}
	if _, err := os.Lstat(keep); err != nil {
		t.Fatalf("keep.go removed: %v", err)
	}
}

func TestCleanOutputsGlobListedTwice(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/*.txt", "generated/*.txt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(hello); !os.IsNotExist(err) {
		t.Fatalf("hello.txt should be removed: %v", err)
	}
}

func TestCleanOutputsGlobBeforeRefusalDeletesNothing(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/*.txt", ".."}})
	if err == nil || !strings.Contains(err.Error(), `clean refuses ".."`) {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(hello)
	if readErr != nil {
		t.Fatalf("hello.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("hello.txt = %q", got)
	}
}

func TestCleanOutputsLiteralOutputStillDeletesHandWrittenFile(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "db.go")
	models := filepath.Join(root, "models.go")
	oidc := filepath.Join(root, "oidc_queries.sql.go")
	for _, path := range []string{db, models, oidc} {
		if err := os.WriteFile(path, []byte("package db\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"db.go", "*_queries.sql.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(db); !os.IsNotExist(err) {
		t.Fatalf("listed db.go should be removed: %v", err)
	}
	if _, err := os.Lstat(oidc); !os.IsNotExist(err) {
		t.Fatalf("query file should be removed: %v", err)
	}
	if _, err := os.Lstat(models); err != nil {
		t.Fatalf("models.go removed: %v", err)
	}
}

func TestCleanOutputsDirectoryStillWipesPackage(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "pkg", "db.go")
	oidc := filepath.Join(root, "pkg", "oidc_queries.sql.go")
	if err := os.MkdirAll(filepath.Dir(db), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{db, oidc} {
		if err := os.WriteFile(path, []byte("package db\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"pkg/", "*_queries.sql.go"}}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{db, oidc} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be removed: %v", filepath.Base(path), err)
		}
	}
	info, err := os.Lstat(filepath.Join(root, "pkg"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("pkg = %v", info.Mode())
	}
}

func TestCleanOutputsGlobRefusesAbsolute(t *testing.T) {
	root := t.TempDir()
	hello := filepath.Join(root, "generated", "hello.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(root, "generated", "*.txt")

	err := cleanOutputs(root, "", config.Group{Outputs: []string{spec}})
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(hello); statErr != nil {
		t.Fatalf("hello.txt removed: %v", statErr)
	}
}

func TestCleanOutputsGlobRefusesYMLConfig(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	configPath := filepath.Join(root, "genguard.yml")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("groups: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"*.yml"}})
	if err == nil || !strings.Contains(err.Error(), "config file") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(hello); statErr != nil {
		t.Fatalf("hello.txt removed: %v", statErr)
	}
	if _, statErr := os.Lstat(configPath); statErr != nil {
		t.Fatalf("config removed: %v", statErr)
	}
}

func TestCleanOutputsGlobRefusesSymlinkFile(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	secret := filepath.Join(outside, "secret.txt")
	keep := filepath.Join(root, "generated", "keep.go")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "generated", "link.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/*.txt"}})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(secret)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "secret\n" {
		t.Fatalf("secret = %q", got)
	}
	if _, statErr := os.Lstat(link); statErr != nil {
		t.Fatalf("link removed: %v", statErr)
	}
	if _, statErr := os.Lstat(keep); statErr != nil {
		t.Fatalf("keep.go removed: %v", statErr)
	}
}

func TestCleanOutputsGlobRefusesSymlinkPrefixWithNoMatches(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "notes", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(keep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	secret := filepath.Join(outside, "secret.txt")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "generated")); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "notes/keep.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"notes/keep.txt", "generated/*.txt"}})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(keep)
	if readErr != nil {
		t.Fatalf("keep.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("keep.txt = %q", got)
	}
	got, readErr = os.ReadFile(secret)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "secret\n" {
		t.Fatalf("secret = %q", got)
	}
}

func TestCleanOutputsGlobRefusesIntermediateSymlink(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "sub", "generated")); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "sub", "keep.go")
	if err := os.WriteFile(keep, []byte("hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "sub/keep.go"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"sub/generated/*.txt"}})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(keep); statErr != nil {
		t.Fatalf("keep.go removed: %v", statErr)
	}
}

func TestCleanOutputsGlobRefusesEmbeddedRepoWithoutDeletingSiblings(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	keep := filepath.Join(root, "generated", "keep.go")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("hand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "generated", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := testutil.InitGitRepo(nested); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/*"}})
	if err == nil || !strings.Contains(err.Error(), "glob matched directory") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(hello)
	if readErr != nil {
		t.Fatalf("hello.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("hello.txt = %q", got)
	}
	if _, statErr := os.Lstat(keep); statErr != nil {
		t.Fatalf("keep.go removed: %v", statErr)
	}
}

func TestCleanOutputsGlobRefusesFileReplacedByDirectory(t *testing.T) {
	root := t.TempDir()
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	hello := filepath.Join(root, "generated", "hello.txt")
	other := filepath.Join(root, "generated", "other.txt")
	if err := os.MkdirAll(filepath.Dir(hello), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hello, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(hello); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(hello, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hello, "nested.txt"), []byte("inside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/*.txt"}})
	if err == nil || !strings.Contains(err.Error(), "glob matched directory") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(other)
	if readErr != nil {
		t.Fatalf("other.txt removed: %v", readErr)
	}
	if string(got) != "keep\n" {
		t.Fatalf("other.txt = %q", got)
	}
	got, readErr = os.ReadFile(filepath.Join(hello, "nested.txt"))
	if readErr != nil {
		t.Fatalf("nested.txt removed: %v", readErr)
	}
	if string(got) != "inside\n" {
		t.Fatalf("nested.txt = %q", got)
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
