package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/regen-check/regen/internal/config"
	"github.com/regen-check/regen/internal/testutil"
)

func TestLoadConfigHappyPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath, err := testutil.WriteRegenConfig(root, "generated/hello.txt", "", "", nil)
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
}

func TestLoadConfigAcceptsRegenYml(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath, err := testutil.WriteRegenConfig(root, "generated/hello.txt", "", "regen.yml", nil)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(cfg.Path) != "regen.yml" {
		t.Fatalf("path = %q", cfg.Path)
	}
}

func TestLoadConfigDefaultGroupName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "regen.yaml")
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
	if _, err := testutil.WriteRegenConfig(root, "", "", "", groups); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(filepath.Join(root, "regen.yaml"))
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
	configPath, err := testutil.WriteRegenConfig(service, "generated/hello.txt", "", "", nil)
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
	configPath := filepath.Join(root, "regen.yaml")
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
			"clean: 1\ngroups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n",
			"clean must be a boolean",
		},
		{
			"groups:\n  - command: python3 scripts/gen.py\n    outputs:\n      - generated/\n    clean: 1\n",
			"groups[0].clean",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.match, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			configPath := filepath.Join(root, "regen.yaml")
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
	if _, err := testutil.WriteRegenConfig(root, "generated/hello.txt", "", "", nil); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, root)

	found, err := config.FindConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(root, "regen.yaml") {
		t.Fatalf("found = %q", found)
	}
}

func TestFindConfigInParentDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := testutil.WriteRegenConfig(root, "generated/hello.txt", "", "", nil); err != nil {
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
	if found != filepath.Join(root, "regen.yaml") {
		t.Fatalf("found = %q", found)
	}
}

func TestFindConfigPrefersNearest(t *testing.T) {
	root := t.TempDir()
	if _, err := testutil.WriteRegenConfig(root, "generated/hello.txt", "", "regen.yaml", nil); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := testutil.WriteRegenConfig(nested, "generated/hello.txt", "", "regen.yml", nil); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, nested)

	found, err := config.FindConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if found != filepath.Join(nested, "regen.yml") {
		t.Fatalf("found = %q", found)
	}
}

func TestLoadConfigGroupClean(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "regen.yaml")
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
	configPath := filepath.Join(root, "regen.yaml")
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
	configPath := filepath.Join(root, "regen.yaml")
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
