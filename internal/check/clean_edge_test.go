package check

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
)

func TestCleanOutputsRejectsFileAsDirectoryPrefix(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(root, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := cleanOutputs(root, "", config.Group{Outputs: []string{"out.txt"}})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %v", err)
	}
	got, readErr := os.ReadFile(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "x" {
		t.Fatalf("file = %q", got)
	}
}

func TestCleanOutputsWrapsRemoveError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "out.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	if f, err := os.OpenFile(filepath.Join(root, "probe"), os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced")
	}

	err := cleanOutputs(root, "", config.Group{Outputs: []string{"out.txt"}})
	if err == nil || !strings.Contains(err.Error(), `clean "out.txt"`) {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, "out.txt")); statErr != nil {
		t.Fatalf("out.txt removed: %v", statErr)
	}
}

func TestCleanOutputsGlobWithoutRepo(t *testing.T) {
	err := cleanOutputs(t.TempDir(), "", config.Group{Outputs: []string{"*.txt"}})
	if err == nil || !strings.Contains(err.Error(), `clean "*.txt"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestCleanOutputsGlobSecondListFails(t *testing.T) {
	root := gitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installGitShim(t, "others-fail")
	err := cleanOutputs(root, "", config.Group{Outputs: []string{"*.txt"}})
	if err == nil || !strings.Contains(err.Error(), "others") {
		t.Fatalf("error = %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, "hello.txt")); statErr != nil {
		t.Fatalf("hello.txt removed: %v", statErr)
	}
}

func TestCleanOutputsGlobDuplicateNames(t *testing.T) {
	root := t.TempDir()
	dup := filepath.Join(root, "dup.go")
	if err := os.WriteFile(dup, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installGitShim(t, "dup-names")
	if err := cleanOutputs(root, "", config.Group{Outputs: []string{"*.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dup); !os.IsNotExist(err) {
		t.Fatalf("dup.go = %v", err)
	}
}

func TestRefuseGlobPrefixSkipsDotAndEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "bar"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []string{"./bar/*.txt", "bar//*.txt", "foo/./bar/*.go"} {
		if err := refuseGlobPrefix(root, spec); err != nil {
			t.Fatalf("%s: %v", spec, err)
		}
	}
}

func TestGitEntryPathCaseOnlyName(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".GIT"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".git")); err == nil {
		t.Skip("volume folds .GIT and .git to the same file")
	}
	got, err := gitEntryPath(filepath.Join(root, ".GIT"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestPathComponentsSkipsDotAndEmpty(t *testing.T) {
	sep := string(filepath.Separator)
	parts, err := pathComponents("foo" + sep + "." + sep + "bar")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parts, []string{"foo", "bar"}) {
		t.Fatalf("parts = %#v", parts)
	}
	parts, err = pathComponents("foo" + sep + sep + "bar")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parts, []string{"foo", "bar"}) {
		t.Fatalf("parts = %#v", parts)
	}
}

func TestRefuseSymlinksInPathRelativeTarget(t *testing.T) {
	root := t.TempDir()
	err := refuseSymlinksInPath("rel", filepath.Join(root, "file"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRemoveCleanTargetRejectsRootEscapeAndRelative(t *testing.T) {
	root := t.TempDir()
	if err := removeCleanTarget(root, root, false); err == nil || !strings.Contains(err.Error(), "clean refuses") {
		t.Fatalf("root: %v", err)
	}
	outside := filepath.Join(root, "..", "outside")
	if err := removeCleanTarget(root, outside, false); err == nil || !strings.Contains(err.Error(), "clean refuses") {
		t.Fatalf("escape: %v", err)
	}
	if err := removeCleanTarget("rel", filepath.Join(root, "file"), false); err == nil {
		t.Fatal("expected rel error")
	}
}

func TestRemoveCleanTargetMissingPaths(t *testing.T) {
	root := t.TempDir()
	missingFile := filepath.Join(root, "missing.txt")
	if err := removeCleanTarget(root, missingFile, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(missingFile); !os.IsNotExist(err) {
		t.Fatalf("missing file = %v", err)
	}

	nested := filepath.Join(root, "missing", "file.txt")
	if err := removeCleanTarget(root, nested, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "missing")); !os.IsNotExist(err) {
		t.Fatalf("missing parent = %v", err)
	}

	if err := removePinned(root, nil, false); err != nil {
		t.Fatal(err)
	}
}

func TestRemoveCleanTargetFileInTheMiddle(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := removeCleanTarget(root, filepath.Join(file, "child"), true)
	if err == nil {
		t.Fatal("expected error")
	}
	got, readErr := os.ReadFile(file)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "x" {
		t.Fatalf("file = %q", got)
	}
}

func TestRemoveCleanTargetCreatesDirectMissingDir(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "generated")
	if err := removeCleanTarget(root, target, true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestRemoveCleanTargetReplacesFileWithDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "generated")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeCleanTarget(root, target, true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("mode = %v", info.Mode())
	}
}

func TestRemoveCleanTargetRemovesNestedDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "generated", "sub", "a.txt")
	sibling := filepath.Join(root, "generated", "b.txt")
	if err := os.MkdirAll(filepath.Dir(nested), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{nested, sibling} {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	target := filepath.Join(root, "generated")
	if err := removeCleanTarget(root, target, true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("mode = %v", info.Mode())
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("entries = %v", entries)
	}
}

func TestRemoveCleanTargetLongName(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("a", 256)
	err := removeCleanTarget(root, filepath.Join(root, long), false)
	if err == nil {
		t.Fatal("expected error")
	}
}
