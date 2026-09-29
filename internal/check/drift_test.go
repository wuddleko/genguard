package check

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestDriftDiffDeduplicatesPaths(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "name.txt"), []byte("genguard\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "name.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "rename"); err != nil {
		t.Fatal(err)
	}
	gen := exec.Command("python3", "scripts/gen.py")
	gen.Dir = root
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generator: %v: %s", err, out)
	}

	drifts := []Drift{
		{Group: "greeting", Path: "generated/hello.txt", Kind: "modified"},
		{Group: "greeting", Path: "generated/hello.txt", Kind: "modified"},
	}
	diff, err := driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(diff, "diff --git") != 1 {
		t.Fatalf("diff = %q", diff)
	}
}

func TestDriftDiffMissingShowsDeletion(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "generated", "hello.txt")); err != nil {
		t.Fatal(err)
	}

	diff, err := driftDiff(root, []Drift{
		{Group: "greeting", Path: "generated/hello.txt", Kind: "missing"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "deleted file mode") && !strings.Contains(diff, "--- a/generated/hello.txt") {
		t.Fatalf("diff = %q", diff)
	}
}

func TestDriftDiffUntrackedDirectoryMessage(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "generated", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "child.txt"), []byte("child\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	diff, err := driftDiff(root, []Drift{
		{Group: "greeting", Path: "generated/nested", Kind: "untracked"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff != "Untracked generated file: generated/nested" {
		t.Fatalf("diff = %q", diff)
	}
}

func TestDriftForGroupSkipsMissingCheckForGlob(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/*.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	group := config.Group{
		Name:    "greeting",
		Command: "python3 scripts/gen.py",
		Outputs: []string{"generated/*.txt"},
	}
	drifts, err := groupDrift(root, group)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 0 {
		t.Fatalf("drifts = %v", drifts)
	}
}

func TestDriftForGroupSkipsMissingCheckForDirectory(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	group := config.Group{
		Name:    "greeting",
		Command: "python3 scripts/gen.py",
		Outputs: []string{"generated/"},
	}
	drifts, err := groupDrift(root, group)
	if err != nil {
		t.Fatal(err)
	}
	for _, drift := range drifts {
		if drift.Kind == "missing" {
			t.Fatalf("unexpected missing drift: %+v", drift)
		}
	}
}

func TestDriftForGroupReportsMissingFileSpec(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	configPath, err := testutil.WriteGenguardConfig(root, "generated/missing.txt", "", "", []testutil.GroupSpec{
		{Name: "greeting", Command: "true", Outputs: []string{"generated/missing.txt"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}

	drifts, err := groupDrift(root, cfg.Groups[0])
	if err != nil {
		t.Fatal(err)
	}
	want := Drift{Group: "greeting", Path: "generated/missing.txt", Kind: "missing"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %v, want [%+v]", drifts, want)
	}
}

func TestDriftForGroupCollapsesEquivalentPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "hello.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drifts, err := groupDrift(root, config.Group{
		Name:    "g",
		Outputs: []string{"./hello.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantModified := Drift{Group: "g", Path: "hello.txt", Kind: "modified"}
	if len(drifts) != 1 || drifts[0] != wantModified {
		t.Fatalf("modified drifts = %v, want [%+v]", drifts, wantModified)
	}

	if err := os.Remove(filepath.Join(root, "hello.txt")); err != nil {
		t.Fatal(err)
	}
	wantMissing := Drift{Group: "g", Path: "hello.txt", Kind: "missing"}
	for _, outputs := range [][]string{
		{"./hello.txt"},
		{"foo/../hello.txt"},
		{"./hello.txt", "foo/../hello.txt"},
		{"hello.txt", "./hello.txt"},
	} {
		drifts, err = groupDrift(root, config.Group{Name: "g", Outputs: outputs})
		if err != nil {
			t.Fatal(err)
		}
		if len(drifts) != 1 || drifts[0] != wantMissing {
			t.Fatalf("outputs %q drifts = %v, want [%+v]", outputs, drifts, wantMissing)
		}
	}
	diff, err := driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(diff, "diff --git") != 1 {
		t.Fatalf("diff = %q", diff)
	}
}

func TestDriftForGroupHandlesNewlineInFilename(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	rel := "generated/hello\nworld.txt"
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.SkipIfFilenameRejected(t, filepath.Dir(path), "hello\nworld.txt")
	if err := os.WriteFile(path, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, rel, "true", "", []testutil.GroupSpec{
		{Name: "greeting", Command: "true", Outputs: []string{"generated/"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drifts, err := groupDrift(root, config.Group{
		Name:    "greeting",
		Command: "true",
		Outputs: []string{"generated/"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Drift{Group: "greeting", Path: rel, Kind: "modified"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %v, want [%+v]", drifts, want)
	}
}

func TestDriftForGroupSubdirectoryUsesRepoPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	api := filepath.Join(root, "api")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, "out.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	group := config.Group{Name: "api", Outputs: []string{"out.txt", "extra.txt"}}
	drifts, err := groupDrift(api, group)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 2 {
		t.Fatalf("drifts = %+v", drifts)
	}
	if drifts[0] != (Drift{Group: "api", Path: "api/out.txt", Kind: "modified"}) {
		t.Fatalf("modified = %+v", drifts[0])
	}
	if drifts[1] != (Drift{Group: "api", Path: "api/extra.txt", Kind: "untracked"}) {
		t.Fatalf("untracked = %+v", drifts[1])
	}
	diff, err := driftDiff(root, drifts[:1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "diff --git") || !strings.Contains(diff, "+new") {
		t.Fatalf("diff = %q", diff)
	}

	if err := os.Remove(filepath.Join(api, "out.txt")); err != nil {
		t.Fatal(err)
	}
	drifts, err = groupDrift(api, config.Group{Name: "api", Outputs: []string{"out.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0] != (Drift{Group: "api", Path: "api/out.txt", Kind: "missing"}) {
		t.Fatalf("deleted = %+v", drifts)
	}
}

func TestDriftForGroupParentPathspec(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	api := filepath.Join(root, "api")
	web := filepath.Join(root, "web")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "out.txt"), []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "config", "diff.relative", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "out.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drifts, err := groupDrift(api, config.Group{Name: "api", Outputs: []string{"../web/out.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0] != (Drift{Group: "api", Path: "web/out.txt", Kind: "modified"}) {
		t.Fatalf("modified = %+v", drifts)
	}
	diff, err := driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "diff --git") || !strings.Contains(diff, "+new") {
		t.Fatalf("diff = %q", diff)
	}

	if err := os.WriteFile(filepath.Join(web, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	drifts, err = groupDrift(api, config.Group{Name: "api", Outputs: []string{"../web/extra.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0] != (Drift{Group: "api", Path: "web/extra.txt", Kind: "untracked"}) {
		t.Fatalf("untracked = %+v", drifts)
	}
	diff, err = driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "diff --git") || !strings.Contains(diff, "+extra") {
		t.Fatalf("diff = %q", diff)
	}

	if err := os.Remove(filepath.Join(web, "out.txt")); err != nil {
		t.Fatal(err)
	}
	drifts, err = groupDrift(api, config.Group{Name: "api", Outputs: []string{"../web/out.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0] != (Drift{Group: "api", Path: "web/out.txt", Kind: "missing"}) {
		t.Fatalf("deleted = %+v", drifts)
	}
	diff, err = driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "deleted file") {
		t.Fatalf("diff = %q", diff)
	}
}

func TestDriftForGroupIgnoresCRLFRenormalizeWarning(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	same := filepath.Join(root, "generated", "same.txt")
	changed := filepath.Join(root, "generated", "changed.txt")
	if err := os.MkdirAll(filepath.Dir(same), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(same, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "config", "core.autocrlf", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageFile(t, same)

	warn := gitDiffStderr(t, root, "generated/same.txt", "generated/changed.txt")
	if !strings.Contains(warn, "LF will be replaced by CRLF") {
		t.Fatalf("git diff stderr = %q, want a CRLF renormalize warning", warn)
	}
	// The probe refreshed the stat cache. Age the file again before the check.
	ageFile(t, same)

	group := config.Group{Name: "greeting", Outputs: []string{"generated/same.txt", "generated/changed.txt"}}
	drifts, err := groupDrift(root, group)
	if err != nil {
		t.Fatal(err)
	}
	want := Drift{Group: "greeting", Path: "generated/changed.txt", Kind: "modified"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %+v, want [%+v]", drifts, want)
	}
	diff, err := driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(diff, "LF will be replaced") {
		t.Fatalf("diff contains CRLF warning: %q", diff)
	}
	if !strings.Contains(diff, "+new") {
		t.Fatalf("diff = %q", diff)
	}

	ageFile(t, same)
	drifts, err = groupDrift(root, config.Group{
		Name:    "greeting",
		Outputs: []string{"generated/same.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 0 {
		t.Fatalf("clean file drifts = %+v", drifts)
	}
}

func TestDriftForGroupReportsGitFatal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/missing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := groupDrift(root, config.Group{Name: "greeting", Outputs: []string{"file.txt"}})
	if err == nil || !strings.Contains(err.Error(), "fatal:") {
		t.Fatalf("error = %v, want git fatal text", err)
	}
}

func TestDriftDiffUntrackedIgnoresCRLFRenormalizeWarning(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "--allow-empty", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "config", "core.autocrlf", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "generated", "new.txt")
	if err := os.WriteFile(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageFile(t, path)
	cmd := exec.Command("git", "-C", root, "diff", "--no-index", "--", os.DevNull, "generated/new.txt")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("git diff --no-index exited 0, want 1")
	}
	if !strings.Contains(stderr.String(), "LF will be replaced by CRLF") {
		t.Fatalf("stderr = %q, want a CRLF warning", stderr.String())
	}
	ageFile(t, path)

	drifts, err := groupDrift(root, config.Group{Name: "greeting", Outputs: []string{"generated/new.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	want := Drift{Group: "greeting", Path: "generated/new.txt", Kind: "untracked"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %+v, want [%+v]", drifts, want)
	}
	text, err := driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "LF will be replaced by CRLF") {
		t.Fatalf("diff = %q, warning leaked into the patch", text)
	}
	if !strings.Contains(text, "+new") {
		t.Fatalf("diff = %q, want the untracked file contents", text)
	}
}

func TestDriftDiffReportsGitFatal(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "generated"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "generated", "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "generated/hello.txt"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte("ref: refs/heads/missing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := driftDiff(root, []Drift{{Group: "greeting", Path: "generated/hello.txt", Kind: "modified"}})
	if err == nil || !strings.Contains(err.Error(), "fatal:") {
		t.Fatalf("modified error = %v, want git fatal text", err)
	}
	_, err = driftDiff(root, []Drift{{Group: "greeting", Path: "generated/hello.txt", Kind: "missing"}})
	if err == nil || !strings.Contains(err.Error(), "fatal:") {
		t.Fatalf("missing error = %v, want git fatal text", err)
	}

	extra := filepath.Join(root, "generated", "extra.txt")
	if err := os.WriteFile(extra, []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := driftDiff(root, []Drift{{Group: "greeting", Path: "generated/extra.txt", Kind: "untracked"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "fatal:") || !strings.Contains(text, "+extra") {
		t.Fatalf("diff = %q, want the untracked patch without the broken HEAD", text)
	}
}

func TestDriftDiffMissingIgnoresCRLFRenormalizeWarning(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	same := filepath.Join(root, "generated", "same.txt")
	gone := filepath.Join(root, "generated", "gone.txt")
	if err := os.MkdirAll(filepath.Dir(same), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(same, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gone, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "config", "core.autocrlf", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	ageFile(t, same)
	warn := gitDiffStderr(t, root, "generated/same.txt", "generated/gone.txt")
	if !strings.Contains(warn, "LF will be replaced by CRLF") {
		t.Fatalf("git diff stderr = %q, want a CRLF renormalize warning", warn)
	}
	ageFile(t, same)

	group := config.Group{Name: "greeting", Outputs: []string{"generated/same.txt", "generated/gone.txt"}}
	drifts, err := groupDrift(root, group)
	if err != nil {
		t.Fatal(err)
	}
	want := Drift{Group: "greeting", Path: "generated/gone.txt", Kind: "missing"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %+v, want [%+v]", drifts, want)
	}
	diff, err := driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(diff, "LF will be replaced") {
		t.Fatalf("diff contains CRLF warning: %q", diff)
	}
	if !strings.Contains(diff, "-hello") {
		t.Fatalf("diff = %q, want the deletion", diff)
	}
}

func TestDriftForGroupSubdirectoryIgnoresCRLFRenormalizeWarning(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	api := filepath.Join(root, "api")
	if err := testutil.InitGitRepo(root); err != nil {
		t.Fatal(err)
	}
	same := filepath.Join(api, "same.txt")
	changed := filepath.Join(api, "changed.txt")
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(same, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "commit", "-m", "seed"); err != nil {
		t.Fatal(err)
	}
	if err := testutil.Git(root, "config", "core.autocrlf", "true"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(api, "extra.txt")
	if err := os.WriteFile(extra, []byte("extra\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageFile(t, same)
	ageFile(t, extra)
	warn := gitDiffStderr(t, api, "same.txt", "changed.txt")
	if !strings.Contains(warn, "LF will be replaced by CRLF") {
		t.Fatalf("git diff stderr = %q, want a CRLF renormalize warning", warn)
	}
	cmd := exec.Command("git", "-C", api, "diff", "--no-index", "--", os.DevNull, "extra.txt")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("git diff --no-index exited 0, want 1")
	}
	if !strings.Contains(stderr.String(), "LF will be replaced by CRLF") {
		t.Fatalf("stderr = %q, want a CRLF warning", stderr.String())
	}
	ageFile(t, same)
	ageFile(t, extra)

	group := config.Group{Name: "api", Outputs: []string{"same.txt", "changed.txt", "extra.txt"}}
	drifts, err := groupDrift(api, group)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 2 {
		t.Fatalf("drifts = %+v", drifts)
	}
	if drifts[0] != (Drift{Group: "api", Path: "api/changed.txt", Kind: "modified"}) {
		t.Fatalf("modified = %+v", drifts[0])
	}
	if drifts[1] != (Drift{Group: "api", Path: "api/extra.txt", Kind: "untracked"}) {
		t.Fatalf("untracked = %+v", drifts[1])
	}
	diff, err := driftDiff(root, drifts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(diff, "LF will be replaced") {
		t.Fatalf("diff contains CRLF warning: %q", diff)
	}
	if !strings.Contains(diff, "+new") || !strings.Contains(diff, "+extra") {
		t.Fatalf("diff = %q", diff)
	}
}

func TestDriftForGroupDeletedTrackedFileIsMissingOnce(t *testing.T) {
	root, err := testutil.MakeRepo(t.TempDir(), "generated/hello.txt", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "generated", "hello.txt")); err != nil {
		t.Fatal(err)
	}

	drifts, err := groupDrift(root, config.Group{
		Name:    "greeting",
		Command: "true",
		Outputs: []string{"generated/hello.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Drift{Group: "greeting", Path: "generated/hello.txt", Kind: "missing"}
	if len(drifts) != 1 || drifts[0] != want {
		t.Fatalf("drifts = %v, want [%+v]", drifts, want)
	}
}

func ageFile(t *testing.T, path string) {
	t.Helper()
	past := time.Now().Add(-2 * time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
}

func gitDiffStderr(t *testing.T, root string, paths ...string) string {
	t.Helper()
	args := append([]string{"-C", root, "diff", "--name-only", "-z", "HEAD", "--"}, paths...)
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return stderr.String()
}
