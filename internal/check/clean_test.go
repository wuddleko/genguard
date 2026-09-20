package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
