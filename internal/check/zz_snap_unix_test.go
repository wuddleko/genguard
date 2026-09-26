//go:build unix

package check

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapPathUnreadableFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(file, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(file, 0o644) })
	if f, err := os.Open(file); err == nil {
		f.Close()
		t.Skip("file permissions are not enforced")
	}
	snap := snapPath(file)
	if snap.missing || snap.hashed {
		t.Fatalf("snap = %+v", snap)
	}
	if samePathSnap(file, pathSnap{mode: snap.mode, size: snap.size, hashed: true, sum: snap.sum}) {
		t.Fatal("unreadable file matched a hashed snapshot")
	}
}
