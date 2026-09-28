package gitx

import (
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
		names := ParseNameList(in)
		for _, name := range names {
			if name == "" || strings.Contains(name, "\x00") {
				t.Fatalf("ParseNameList(%q) produced %q", in, name)
			}
		}
		again := ParseNameList(strings.Join(names, "\x00"))
		if len(names) != len(again) {
			t.Fatalf("ParseNameList(%q) = %#v, again %#v", in, names, again)
		}
		for i := range names {
			if names[i] != again[i] {
				t.Fatalf("ParseNameList(%q) = %#v, again %#v", in, names, again)
			}
		}
	})
}
