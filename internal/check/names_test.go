package check

import (
	"reflect"
	"testing"
)

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
