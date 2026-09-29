package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGlobsOverlap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		a, b string
		want bool
	}{
		{"db.go", "db.go", true},
		{"db.go", "*.sql.go", false},
		{"models.go", "*.sql.go", false},
		{"querier.go", "*.sql.go", false},
		{"foo.sql.go", "*.sql.go", true},
		{"*generated.go", "*models_gen.go", false},
		{"*generated.go", "generated.go", true},
		{"*_templ.go", "page_templ.go", true},
		{"*.pb.go", "*.connect.go", false},
		{"*.txt", "out.go", false},
		{"*.txt", "out.txt", true},
		{"*", "db.go", true},
		{"file[12].txt", "file1.txt", true},
		{"file[12].txt", "file3.txt", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.a+" "+tc.b, func(t *testing.T) {
			t.Parallel()
			if got := globsOverlap(tc.a, tc.b); got != tc.want {
				t.Fatalf("globsOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestPatternsOverlap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cases := []struct {
		a, b string
		want bool
	}{
		{"services/jobs/*.sql.go", "services/jobs/db.go", false},
		{"services/jobs/*.sql.go", "services/jobs/models.go", false},
		{"services/jobs/*.sql.go", "services/jobs/querier.go", false},
		{"services/jobs/*.sql.go", "services/jobs/foo.sql.go", true},
		{"*generated.go", "*models_gen.go", false},
		{"pkg/gen/", "*_templ.go", true},
		{"gen/", "gen/*.txt", true},
		{"gen/*.txt", "gen/out.go", false},
		{"gen/*.txt", "gen/out.txt", true},
		{"proto/**/*.pb.go", "proto/**/*.connect.go", false},
		{"*.gen.go", "internal/postgresql/db/", true},
		{"proto/gen/", "*.sql.go", true},
		{"gen/out.go", "gen/out.go", true},
		{"gen/", "other/out.go", false},
		{"a/*.txt", "b/*.txt", false},
		// Git pathspec wildcards match "/" too.
		{"gen/*.go", "gen/sub/", true},
		{"gen/*.go", "gen/sub/deep.go", true},
		{"gen/sub/", "gen/*.txt", true},
		{"gen?sub/x.txt", "gen/sub/x.txt", true},
		{"gen[/]sub/x.txt", "gen/sub/x.txt", true},
		// A wildcard that matches only a directory name names nothing under it.
		{"gen/su?", "gen/sub/x.txt", false},
		{"proto/**/*.pb.go", "proto/a.pb.go", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.a+" "+tc.b, func(t *testing.T) {
			t.Parallel()
			got := patternsOverlap(compile(dir, tc.a), compile(dir, tc.b))
			if got != tc.want {
				t.Fatalf("overlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestPatternsOverlapDirectoryWithoutSlash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "gen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "db.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		a, b string
		want bool
	}{
		{"gen", "gen/x.go", true},
		{"gen", "gen/sub/", true},
		{"gen", "*.go", true},
		{"db.go", "*.sql.go", false},
		{"missing", "missing/x.go", false},
	}
	if err := os.Symlink(filepath.Join(dir, "gen"), filepath.Join(dir, "link")); err == nil {
		cases = append(cases, struct {
			a, b string
			want bool
		}{"link", "link/x.go", false})
	}
	for _, tc := range cases {
		got := patternsOverlap(compile(dir, tc.a), compile(dir, tc.b))
		if got != tc.want {
			t.Errorf("overlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func compile(dir, spec string) pattern {
	return compileOutput(dir, spec, worktreeIsDir)
}

func TestPatternsOverlapKeepsConfigDirectoryLiteral(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	bracketed := filepath.Join(parent, "svc[12]")
	plain := filepath.Join(parent, "svc1")
	if patternsOverlap(compile(bracketed, "*.go"), compile(plain, "out.go")) {
		t.Fatal("svc[12]/*.go overlapped svc1/out.go")
	}
	if !patternsOverlap(compile(bracketed, "*.go"), compile(bracketed, "sub/out.go")) {
		t.Fatal("svc[12]/*.go missed svc[12]/sub/out.go")
	}
}
