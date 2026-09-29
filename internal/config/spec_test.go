package config

import (
	"reflect"
	"testing"
)

func TestParseSpec(t *testing.T) {
	t.Parallel()
	cases := []struct {
		spec   string
		glob   bool
		dir    bool
		prefix []string
	}{
		{"gen/out.go", false, false, []string{"gen", "out.go"}},
		{"gen/", false, true, []string{"gen"}},
		{"gen/*.go", true, false, []string{"gen"}},
		{"./gen//sub/*.go", true, false, []string{"gen", "sub"}},
		{"*.pb.go", true, false, nil},
		{"proto/**/*.pb.go", true, false, []string{"proto"}},
		{"gen/*/", true, true, []string{"gen"}},
	}
	for _, tc := range cases {
		got := ParseSpec(tc.spec)
		if got.Glob != tc.glob || got.Dir != tc.dir {
			t.Errorf("ParseSpec(%q) = glob %v dir %v, want glob %v dir %v", tc.spec, got.Glob, got.Dir, tc.glob, tc.dir)
		}
		if prefix := got.GlobPrefix(); !reflect.DeepEqual(prefix, tc.prefix) {
			t.Errorf("GlobPrefix(%q) = %q, want %q", tc.spec, prefix, tc.prefix)
		}
	}
}
