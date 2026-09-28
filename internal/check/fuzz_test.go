package check

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzConfigRelativeGitPath(f *testing.F) {
	f.Add("", "generated/hello.txt")
	f.Add("api/", "api/out.txt")
	f.Add("api/", "web/out.txt")
	f.Add("api/sub/", "api/sub/out.txt")
	f.Add("api/sub/", "web/out.txt")
	f.Add("", "generated/hello\nworld.txt")
	f.Fuzz(func(t *testing.T, prefix, gitPath string) {
		got, err := configRelativeGitPath(prefix, gitPath)
		if err != nil {
			return
		}
		base := strings.TrimSuffix(prefix, "/")
		if base == "" {
			base = "."
		}
		joined := filepath.Join(filepath.FromSlash(base), filepath.FromSlash(got))
		rel, err := filepath.Rel(filepath.FromSlash(base), joined)
		if err != nil {
			t.Fatalf("Rel(%q, %q): %v", base, joined, err)
		}
		if filepath.ToSlash(rel) != got {
			t.Fatalf("configRelativeGitPath(%q, %q) = %q, re-rel = %q", prefix, gitPath, got, filepath.ToSlash(rel))
		}
	})
}
