package clean

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestCleanPathsWhenWorkingDirectoryIsGone(t *testing.T) {
	root := t.TempDir()
	testutil.WithoutWorkingDirectory(t)

	if wouldRemove("rel", "other") {
		t.Fatal("relative target")
	}
	if wouldRemove(root, "rel") {
		t.Fatal("relative path")
	}
	if _, _, err := resolveCleanPath("rel", "out.txt", true); err == nil {
		t.Fatal("resolveCleanPath")
	}
	if err := cleanOutputs("rel", "", config.Group{Outputs: []string{"out.txt"}}); err == nil {
		t.Fatal("cleanOutputs")
	}
}

func TestWouldRemoveAcrossWindowsVolumes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("filepath.Rel fails across volumes only on windows")
	}
	if wouldRemove(`C:\a`, `D:\b`) {
		t.Fatal("want false")
	}
}

func TestRefuseFileInPathMissingAndFile(t *testing.T) {
	root := t.TempDir()
	if err := refuseFileInPath(filepath.Join(root, "missing")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := refuseFileInPath(file)
	if err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("error = %v", err)
	}
}

func TestRefuseFileInPathAllowsSymlinkParent(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := refuseFileInPath(link); err != nil {
		t.Fatal(err)
	}
}
