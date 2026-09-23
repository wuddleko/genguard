package testutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const genScript = `from pathlib import Path
root = Path(__file__).resolve().parents[1]
src = (root / "name.txt").read_text(encoding="utf-8").strip()
out = root / "generated"
out.mkdir(exist_ok=True)
(out / "hello.txt").write_text(f"hello {src}\n", encoding="utf-8")
`

type GroupSpec struct {
	Name    string
	Command string
	Outputs []string
	Clean   bool
}

// Chdir changes the process working directory for the rest of the test and
// restores it afterward. Prefer this over t.Chdir so tests compile on Go 1.22.
func Chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	pwd := dir
	if !filepath.IsAbs(pwd) {
		pwd, err = os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PWD", pwd)
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	})
}

func Git(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func InitGitRepo(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := Git(root, "init"); err != nil {
		return err
	}
	if err := Git(root, "config", "user.email", "genguard@example.test"); err != nil {
		return err
	}
	return Git(root, "config", "user.name", "genguard")
}

func WriteGenerator(root string) error {
	scripts := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scripts, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(scripts, "gen.py"), []byte(genScript), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "name.txt"), []byte("world\n"), 0o644)
}

func yamlScalar(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, ":#\n\"'*&!?|>[]{}@`") || strings.Contains(value, " ") {
		return fmt.Sprintf("%q", value)
	}
	switch value {
	case "true", "false", "null", "yes", "no", "on", "off":
		return fmt.Sprintf("%q", value)
	}
	return value
}

func WriteGenguardConfig(root string, outputs, command, configName string, groups []GroupSpec) (string, error) {
	if command == "" {
		command = "python3 scripts/gen.py"
	}
	if groups == nil {
		if outputs == "" {
			outputs = "generated/hello.txt"
		}
		groups = []GroupSpec{{
			Name:    "greeting",
			Command: command,
			Outputs: []string{outputs},
		}}
	}
	if configName == "" {
		configName = "genguard.yaml"
	}

	var b strings.Builder
	b.WriteString("groups:\n")
	for _, group := range groups {
		b.WriteString(fmt.Sprintf("  - name: %s\n", yamlScalar(group.Name)))
		b.WriteString(fmt.Sprintf("    command: %s\n", yamlScalar(group.Command)))
		b.WriteString("    outputs:\n")
		for _, output := range group.Outputs {
			b.WriteString(fmt.Sprintf("      - %s\n", yamlScalar(output)))
		}
		if group.Clean {
			b.WriteString("    clean: true\n")
		}
	}
	path := filepath.Join(root, configName)
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

func MakeRepo(tmp string, outputs, configName string, groups []GroupSpec) (string, error) {
	root := filepath.Join(tmp, "repo")
	if err := InitGitRepo(root); err != nil {
		return "", err
	}
	if err := WriteGenerator(root); err != nil {
		return "", err
	}
	if _, err := WriteGenguardConfig(root, outputs, "", configName, groups); err != nil {
		return "", err
	}
	cmd := exec.Command("python3", "scripts/gen.py")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("seed generator: %w: %s", err, out)
	}
	if err := Git(root, "add", "."); err != nil {
		return "", err
	}
	if err := Git(root, "commit", "-m", "seed"); err != nil {
		return "", err
	}
	return root, nil
}

// SkipIfFilenameRejected skips when dir cannot hold a file named name.
// Windows rejects names that contain a newline.
func SkipIfFilenameRejected(t *testing.T, dir, name string) {
	t.Helper()
	if strings.ContainsAny(name, `/\`) {
		t.Fatalf("name must be a single path element, got %q", name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Skipf("filesystem rejects filename %q: %v", name, err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
