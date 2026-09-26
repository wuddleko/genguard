package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/check/clean"
	"github.com/wuddleko/genguard/internal/config"
)

func TestCleanOutputsGlobSecondListFails(t *testing.T) {
	root := gitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installGitShim(t, "others-fail")
	err := clean.Outputs(root, "", config.Group{Outputs: []string{"*.txt"}})
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
	if err := clean.Outputs(root, "", config.Group{Outputs: []string{"*.go"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dup); !os.IsNotExist(err) {
		t.Fatalf("dup.go = %v", err)
	}
}
