package config

import "testing"

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
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.a+" "+tc.b, func(t *testing.T) {
			t.Parallel()
			got := patternsOverlap(compileOutput(dir, tc.a), compileOutput(dir, tc.b))
			if got != tc.want {
				t.Fatalf("overlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
