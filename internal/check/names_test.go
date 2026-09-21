package check

import (
	"reflect"
	"testing"
)

func TestConfigRelativeGitPath(t *testing.T) {
	tests := []struct {
		prefix  string
		gitPath string
		want    string
	}{
		{prefix: "", gitPath: "generated/hello.txt", want: "generated/hello.txt"},
		{prefix: "api/", gitPath: "api/out.txt", want: "out.txt"},
		{prefix: "api/", gitPath: "web/out.txt", want: "../web/out.txt"},
		{prefix: "api/sub/", gitPath: "api/sub/out.txt", want: "out.txt"},
		{prefix: "api/sub/", gitPath: "web/out.txt", want: "../../web/out.txt"},
		{prefix: "", gitPath: "generated/hello\nworld.txt", want: "generated/hello\nworld.txt"},
	}
	for _, tc := range tests {
		t.Run(tc.prefix+" "+tc.gitPath, func(t *testing.T) {
			got, err := configRelativeGitPath(tc.prefix, tc.gitPath)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("configRelativeGitPath(%q, %q) = %q, want %q", tc.prefix, tc.gitPath, got, tc.want)
			}
		})
	}
}

func TestParseGitNameList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "single", in: "a.go\x00", want: []string{"a.go"}},
		{name: "multiple", in: "a.go\x00b.go\x00", want: []string{"a.go", "b.go"}},
		{name: "newline in name", in: "hello\nworld.go\x00", want: []string{"hello\nworld.go"}},
		{name: "no trailing nul", in: "only.go", want: []string{"only.go"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseGitNameList(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseGitNameList(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
