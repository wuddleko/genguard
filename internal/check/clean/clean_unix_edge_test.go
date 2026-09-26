//go:build unix

package clean

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
	"golang.org/x/sys/unix"
)

func TestCleanUnixPermissionFailures(t *testing.T) {
	if !permissionsAreEnforced(t) {
		t.Skip("directory permissions are not enforced")
	}

	t.Run("glob prefix", func(t *testing.T) {
		root := t.TempDir()
		secret := filepath.Join(root, "secret")
		if err := os.Mkdir(secret, 0o755); err != nil {
			t.Fatal(err)
		}
		lockDir(t, secret)
		err := refuseGlobPrefix(root, "secret/gen/*.txt")
		if err == nil || os.IsNotExist(err) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("walk", func(t *testing.T) {
		root := t.TempDir()
		locked := filepath.Join(root, "generated", "locked")
		if err := os.MkdirAll(locked, 0o755); err != nil {
			t.Fatal(err)
		}
		lockDir(t, locked)
		err := cleanOutputs(root, "", config.Group{Outputs: []string{"generated/"}})
		if err == nil || !strings.Contains(err.Error(), `clean "generated/"`) {
			t.Fatalf("error = %v", err)
		}
		if _, statErr := os.Lstat(locked); statErr != nil {
			t.Fatalf("locked removed: %v", statErr)
		}
	})

	t.Run("lstat target", func(t *testing.T) {
		root := t.TempDir()
		parent := filepath.Join(root, "parent")
		child := filepath.Join(parent, "child")
		if err := os.MkdirAll(child, 0o755); err != nil {
			t.Fatal(err)
		}
		lockDir(t, parent)
		if _, _, err := findProtected(child); err == nil || os.IsNotExist(err) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("git entry", func(t *testing.T) {
		root := t.TempDir()
		gitDir := filepath.Join(root, ".git")
		if err := os.Mkdir(gitDir, 0o755); err != nil {
			t.Fatal(err)
		}
		lockDir(t, root)
		if _, _, err := foldedEntry(gitDir, ".git"); err == nil || os.IsNotExist(err) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unreadable directory", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "generated")
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		lockDir(t, target)
		err := removeCleanTarget(root, target, true)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unreadable child", func(t *testing.T) {
		root := t.TempDir()
		locked := filepath.Join(root, "generated", "sub", "locked")
		if err := os.MkdirAll(locked, 0o755); err != nil {
			t.Fatal(err)
		}
		lockDir(t, locked)
		err := removeCleanTarget(root, filepath.Join(root, "generated"), true)
		if err == nil {
			t.Fatal("expected error")
		}
		if _, statErr := os.Lstat(filepath.Dir(locked)); statErr != nil {
			t.Fatalf("parent removed: %v", statErr)
		}
	})
}

func TestRemovePinnedOpenFailure(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removePinned(file, []string{"child"}, false); err == nil {
		t.Fatal("expected error")
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "x" {
		t.Fatalf("file = %q", got)
	}
}

func TestRemoveCleanTargetCannotReplaceLockedFile(t *testing.T) {
	if !permissionsAreEnforced(t) {
		t.Skip("directory permissions are not enforced")
	}
	root := t.TempDir()
	target := filepath.Join(root, "generated")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	err := removeCleanTarget(root, target, true)
	if err == nil {
		t.Fatal("expected error")
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "x" {
		t.Fatalf("file = %q", got)
	}
}

func TestRefuseFileInPathUnreadableParent(t *testing.T) {
	if !permissionsAreEnforced(t) {
		t.Skip("directory permissions are not enforced")
	}
	root := t.TempDir()
	secret := filepath.Join(root, "secret")
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	lockDir(t, secret)
	err := refuseFileInPath(filepath.Join(secret, "child"))
	if err == nil || os.IsNotExist(err) {
		t.Fatalf("error = %v", err)
	}
}

func TestRemoveCleanTargetParentNotWritable(t *testing.T) {
	if !permissionsAreEnforced(t) {
		t.Skip("directory permissions are not enforced")
	}
	root := t.TempDir()
	target := filepath.Join(root, "generated")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	err := removeCleanTarget(root, target, true)
	if err == nil {
		t.Fatal("expected error")
	}
	info, statErr := os.Lstat(target)
	if statErr != nil {
		t.Fatalf("generated removed: %v", statErr)
	}
	if !info.IsDir() {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestRemoveCleanTargetBusyDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "generated")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(target); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := removeCleanTarget(root, target, true); err == nil {
		t.Skip("platform removed the current directory")
	}
}

func TestMkdirAllAtLongName(t *testing.T) {
	fd := openDir(t, t.TempDir())
	if err := mkdirAllAt(fd, nil, []string{strings.Repeat("a", 256)}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCleanUnixOpenRaces(t *testing.T) {
	root := t.TempDir()
	name := "generated"
	if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "keep"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fd := openDir(t, root)

	t.Run("directory became a symlink", func(t *testing.T) {
		prev := openNoFollow
		openNoFollow = func(int, string) (int, error) { return -1, unix.ELOOP }
		t.Cleanup(func() { openNoFollow = prev })
		err := removeNameAt(fd, name, true, name)
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("removeNameAt: %v", err)
		}
		err = mkdirAllAt(fd, nil, []string{name, "child"})
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("mkdirAllAt: %v", err)
		}
		err = removeAllAt(fd, name)
		if err == nil {
			t.Fatal("removeAllAt unlinked a directory")
		}
		if _, statErr := os.Lstat(filepath.Join(root, name)); statErr != nil {
			t.Fatalf("generated: %v", statErr)
		}
	})

	t.Run("directory disappeared", func(t *testing.T) {
		prev := openNoFollow
		openNoFollow = func(int, string) (int, error) { return -1, unix.ENOENT }
		t.Cleanup(func() { openNoFollow = prev })
		if err := removeAllAt(fd, name); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("dot entries", func(t *testing.T) {
		prev := listDir
		listDir = func(int) ([]string, error) {
			return []string{".", "..", "keep"}, nil
		}
		t.Cleanup(func() { listDir = prev })
		if err := clearDirFd(fd); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(filepath.Join(root, "keep")); !os.IsNotExist(err) {
			t.Fatalf("keep = %v", err)
		}
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
			t.Fatalf("generated = %v", err)
		}
	})
}

func TestCleanUnixDirectoryFdEdges(t *testing.T) {
	root := t.TempDir()
	fd := openDir(t, root)
	if err := removeAllAt(fd, "missing"); err != nil {
		t.Fatal(err)
	}
	if err := removeAllAt(fd, strings.Repeat("n", 256)); err == nil {
		t.Fatal("expected long name error")
	}
	if err := clearDirFd(-1); err == nil {
		t.Fatal("expected bad fd error")
	}
	if ignoreNotExist(unix.ENOENT) != nil {
		t.Fatal("enoent")
	}
	if ignoreNotExist(nil) != nil {
		t.Fatal("nil")
	}
	if !errors.Is(ignoreNotExist(unix.EIO), unix.EIO) {
		t.Fatal("eio")
	}
}

func TestMkdirAllAtRefusesObstacles(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("target", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	fd := openDir(t, root)
	if err := mkdirAllAt(fd, nil, []string{"link"}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink: %v", err)
	}
	if err := mkdirAllAt(fd, nil, []string{"file", "child"}); !errors.Is(err, unix.ENOTDIR) {
		t.Fatalf("file: %v", err)
	}
	if !permissionsAreEnforced(t) {
		t.Skip("directory permissions are not enforced")
	}
	lockDir(t, locked)
	if err := mkdirAllAt(fd, nil, []string{"locked", "child"}); err == nil || strings.Contains(err.Error(), "symlink") {
		t.Fatalf("locked: %v", err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := mkdirAllAt(fd, nil, []string{"created"}); err == nil {
		t.Fatal("mkdir on a read-only directory")
	}
}

func permissionsAreEnforced(t *testing.T) bool {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	f, err := os.Open(dir)
	if err == nil {
		f.Close()
		return false
	}
	return true
}

func lockDir(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
}

func openDir(t *testing.T, path string) int {
	t.Helper()
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = unix.Close(fd) })
	return fd
}
