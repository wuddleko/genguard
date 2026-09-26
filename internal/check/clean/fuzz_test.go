package clean

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzResolveCleanPath(f *testing.F) {
	root := f.TempDir()
	absRoot, err := filepath.Abs(root)
	if err != nil {
		f.Fatal(err)
	}
	f.Add("generated/")
	f.Add("generated/hello.txt")
	f.Add("")
	f.Add(".")
	f.Add("./")
	f.Add("..")
	f.Add("../")
	f.Add("foo/../..")
	f.Add("generated/..")
	f.Add("generated/*.txt")
	f.Add("*.go")
	f.Add("/etc/passwd")
	f.Add("foo/../bar")
	f.Fuzz(func(t *testing.T, spec string) {
		trimmed := strings.TrimSpace(spec)
		for _, rejectGlob := range []bool{false, true} {
			target, _, err := resolveCleanPath(absRoot, spec, rejectGlob)
			if err != nil {
				continue
			}
			if trimmed == "" {
				t.Fatalf("accepted empty spec %q", spec)
			}
			if filepath.IsAbs(trimmed) {
				t.Fatalf("accepted absolute spec %q -> %s", spec, target)
			}
			if rejectGlob && isGlob(trimmed) {
				t.Fatalf("accepted glob spec %q -> %s", spec, target)
			}
			rel, relErr := filepath.Rel(absRoot, target)
			if relErr != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Fatalf("resolveCleanPath(%q) = %s escaped %s: %v", spec, target, absRoot, relErr)
			}
		}

		err := validateGlobSpec(spec)
		if err != nil {
			return
		}
		if spec == "" {
			t.Fatal("validateGlobSpec accepted an empty spec")
		}
		if filepath.IsAbs(spec) {
			t.Fatalf("validateGlobSpec accepted absolute spec %q", spec)
		}
		for _, part := range strings.Split(filepath.ToSlash(spec), "/") {
			if part == ".." {
				t.Fatalf("validateGlobSpec accepted %q", spec)
			}
		}
	})
}
