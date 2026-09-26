package check

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzParseGitNameList(f *testing.F) {
	f.Add("")
	f.Add("a.go\x00")
	f.Add("a.go\x00b.go\x00")
	f.Add("hello\nworld.go\x00")
	f.Add("only.go")
	f.Add("\x00\x00")
	f.Fuzz(func(t *testing.T, in string) {
		names := parseGitNameList(in)
		for _, name := range names {
			if name == "" || strings.Contains(name, "\x00") {
				t.Fatalf("parseGitNameList(%q) produced %q", in, name)
			}
		}
		again := parseGitNameList(strings.Join(names, "\x00"))
		if len(names) != len(again) {
			t.Fatalf("parseGitNameList(%q) = %#v, again %#v", in, names, again)
		}
		for i := range names {
			if names[i] != again[i] {
				t.Fatalf("parseGitNameList(%q) = %#v, again %#v", in, names, again)
			}
		}
	})
}

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
