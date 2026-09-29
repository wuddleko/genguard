package pathx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelInsideResolved(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, root, path, want string
		ok                     bool
	}{
		{"lexical", real, filepath.Join(real, "sub"), "sub", true},
		{"root through a link", link, filepath.Join(resolved, "sub", "gone.yaml"), filepath.Join("sub", "gone.yaml"), true},
		{"both through links", real, filepath.Join(link, "sub"), "sub", true},
		{"outside", real, t.TempDir(), "", false},
		{"missing outside", real, filepath.Join(t.TempDir(), "gone"), "", false},
	} {
		rel, ok := RelInsideResolved(tc.root, tc.path)
		if ok != tc.ok || rel != tc.want {
			t.Errorf("%s: RelInsideResolved(%q, %q) = %q, %v; want %q, %v", tc.name, tc.root, tc.path, rel, ok, tc.want, tc.ok)
		}
	}
}
