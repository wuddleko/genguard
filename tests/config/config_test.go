package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestLoadConfigHappyPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path != configPath {
		t.Fatalf("path = %q, want %q", cfg.Path, configPath)
	}
	if cfg.Root() != root {
		t.Fatalf("root = %q, want %q", cfg.Root(), root)
	}
	if len(cfg.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(cfg.Groups))
	}
	group := cfg.Groups[0]
	if group.Name != "greeting" || group.Command != "python3 scripts/gen.py" {
		t.Fatalf("group = %+v", group)
	}
	if group.Clean {
		t.Fatal("clean defaults to false")
	}
	if len(group.Outputs) != 1 || group.Outputs[0] != "generated/hello.txt" {
		t.Fatalf("outputs = %v", group.Outputs)
	}
	if group.Inputs != nil {
		t.Fatalf("inputs = %v, want none", group.Inputs)
	}
}

func TestLoadConfigRelativePathIsAbsolute(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "repo")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "custom.yaml")
	content := "groups:\n  - command: echo hi\n    outputs:\n      - out.txt\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Error(err)
		}
	})

	cfg, err := config.LoadConfig(filepath.Join("repo", "custom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(cfg.Path) {
		t.Fatalf("path = %q", cfg.Path)
	}
	if !samePath(t, cfg.Path, configPath) {
		t.Fatalf("path = %q, want %q", cfg.Path, configPath)
	}
	if !samePath(t, cfg.Root(), dir) {
		t.Fatalf("root = %q, want %q", cfg.Root(), dir)
	}
}

func samePath(t *testing.T, got, want string) bool {
	t.Helper()
	a, err := os.Stat(got)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(a, b)
}

func TestLoadConfigInputs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	content := "groups:\n" +
		"  - name: sqlc\n" +
		"    command: sqlc generate\n" +
		"    inputs:\n" +
		"      - queries/\n" +
		"      - sqlc.yaml\n" +
		"      - '   '\n" +
		"    outputs:\n" +
		"      - internal/db/\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(cfg.Groups[0].Inputs, ",")
	if got != "queries/,sqlc.yaml" {
		t.Fatalf("inputs = %q", got)
	}
}

func TestLoadConfigAcceptsGenguardYml(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "", "genguard.yml", nil)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(cfg.Path) != "genguard.yml" {
		t.Fatalf("path = %q", cfg.Path)
	}
}

func TestLoadConfigDefaultGroupName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	content := "groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Groups[0].Name != "groups[0]" {
		t.Fatalf("name = %q", cfg.Groups[0].Name)
	}
}

func TestLoadConfigMultipleGroups(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	groups := []testutil.GroupSpec{
		{Name: "one", Command: "make one", Outputs: []string{"out/one/"}},
		{Name: "two", Command: "make two", Outputs: []string{"out/two.txt"}},
	}
	if _, err := testutil.WriteGenguardConfig(root, "", "", "", groups); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "genguard.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(cfg.Groups))
	for i, group := range cfg.Groups {
		names[i] = group.Name
	}
	if strings.Join(names, ",") != "one,two" {
		t.Fatalf("names = %v", names)
	}
}

func TestLoadConfigRootIsConfigParent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	service := filepath.Join(root, "service")
	if err := os.MkdirAll(service, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath, err := testutil.WriteGenguardConfig(service, "generated/hello.txt", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Root() != service {
		t.Fatalf("root = %q, want %q", cfg.Root(), service)
	}
}

func TestLoadConfigFiltersBlankOutputEntries(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	content := "groups:\n" +
		"  - name: greeting\n" +
		"    command: python3 scripts/gen.py\n" +
		"    outputs:\n" +
		"      - generated/hello.txt\n" +
		"      - '   '\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Groups[0].Outputs) != 1 || cfg.Groups[0].Outputs[0] != "generated/hello.txt" {
		t.Fatalf("outputs = %v", cfg.Groups[0].Outputs)
	}
}

func TestLoadConfigValidationErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		content string
		match   string
	}{
		{"true\n", "must be a mapping"},
		{"hello\n", "must be a mapping"},
		{"[]", "non-empty"},
		{"groups: []", "non-empty"},
		{"groups:\n  - not-a-mapping\n", "mapping"},
		{"groups:\n  - name: greeting\n    outputs:\n      - generated/\n", "command"},
		{"groups:\n  - name: greeting\n    command: python3 scripts/gen.py\n", "outputs"},
		{"groups:\n  - name: greeting\n    command: ''\n    outputs:\n      - generated/\n", "command"},
		{"groups:\n  - name: greeting\n    command: python3 scripts/gen.py\n    outputs: []\n", "outputs"},
		{
			"groups:\n  - name: greeting\n    command: python3 scripts/gen.py\n    outputs:\n      - '  '\n",
			"no usable paths",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs: generated/\n",
			"'outputs' must be a list",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n      - null\n",
			"groups[0].outputs[1] must be a string",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - 1\n",
			"groups[0].outputs[0] must be a string",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n    inputs:\n      - true\n",
			"groups[0].inputs[0] must be a string",
		},
		{
			"groups:\n  - name: 1\n    command: python3 scripts/gen.py\n    outputs:\n      - generated/\n",
			"groups[0].name must be a string",
		},
		{
			"clean: 1\ngroups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n",
			"clean must be a boolean",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n    clean: 1\n",
			"groups[0].clean",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n    inputs: []\n",
			"non-empty 'inputs' list",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n    inputs:\n",
			"non-empty 'inputs' list",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n    inputs:\n      - '  '\n",
			"no usable paths",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n    inputs: queries/\n",
			"'inputs' must be a list",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n    inptus:\n      - queries/\n",
			`unknown key "inptus"`,
		},
		{
			"typo: true\ngroups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n",
			`genguard.yaml: unknown key "typo"`,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.match, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			configPath := filepath.Join(root, "genguard.yaml")
			if err := os.WriteFile(configPath, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := config.LoadConfig(configPath)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.match) {
				t.Fatalf("error = %q, want substring %q", err, tc.match)
			}
		})
	}
}

func TestFindConfigInCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "", "", nil); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, root)

	found, err := config.FindConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(root, "genguard.yaml") {
		t.Fatalf("found = %q", found)
	}
}

func TestFindConfigInParentDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "", "", nil); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, nested)

	found, err := config.FindConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(root, "genguard.yaml") {
		t.Fatalf("found = %q", found)
	}
}

func TestFindConfigPrefersNearest(t *testing.T) {
	root := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "", "genguard.yaml", nil); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(nested, "generated/hello.txt", "", "genguard.yml", nil); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, nested)

	found, err := config.FindConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(nested, "genguard.yml") {
		t.Fatalf("found = %q", found)
	}
}

func TestLoadConfigGroupClean(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	content := "groups:\n" +
		"  - name: greeting\n" +
		"    command: python3 scripts/gen.py\n" +
		"    outputs:\n" +
		"      - generated/\n" +
		"    clean: true\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Groups[0].Clean {
		t.Fatal("expected clean true")
	}
}

func TestLoadConfigTopLevelClean(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	content := "clean: true\n" +
		"groups:\n" +
		"  - name: greeting\n" +
		"    command: python3 scripts/gen.py\n" +
		"    outputs:\n" +
		"      - generated/\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Groups[0].Clean {
		t.Fatal("expected inherited clean true")
	}
}

func TestLoadConfigGroupCleanOverridesTopLevel(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "genguard.yaml")
	content := "clean: true\n" +
		"groups:\n" +
		"  - name: greeting\n" +
		"    command: python3 scripts/gen.py\n" +
		"    outputs:\n" +
		"      - generated/\n" +
		"    clean: false\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Groups[0].Clean {
		t.Fatal("expected group override clean false")
	}
}

func TestExampleYAMLTemplatesLoad(t *testing.T) {
	root := filepath.Join("..", "..")
	matches, err := filepath.Glob(filepath.Join(root, "examples", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("expected example yaml templates under examples/")
	}
	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			cfg, err := config.LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(cfg.Groups) == 0 {
				t.Fatal("expected at least one group")
			}
		})
	}
}

func TestFindConfigRejectsBothNames(t *testing.T) {
	root := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "", "genguard.yaml", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "generated/hello.txt", "", "genguard.yml", nil); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, nested)

	_, err := config.FindConfig("")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "both genguard.yaml and genguard.yml") || !strings.Contains(err.Error(), root) {
		t.Fatalf("error = %q", err)
	}
}

func TestFindConfigIgnoresDirectoryNamedYaml(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "genguard.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "genguard.yml", nil); err != nil {
		t.Fatal(err)
	}

	found, err := config.FindConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(root, "genguard.yml") {
		t.Fatalf("found = %q", found)
	}
}

func TestFindConfigStatError(t *testing.T) {
	root := t.TempDir()
	if _, err := testutil.WriteGenguardConfig(root, "gen/", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o755) })
	if f, err := os.Open(root); err == nil {
		f.Close()
		t.Skip("directory permissions are not enforced")
	}

	_, err := config.FindConfig(root)
	if err == nil {
		t.Fatal("expected error")
	}
	if os.IsNotExist(err) {
		t.Fatalf("error = %v, want a stat failure", err)
	}
}

func TestFindConfigReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)

	found, err := config.FindConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if found != "" {
		t.Fatalf("found = %q, want empty", found)
	}
}
