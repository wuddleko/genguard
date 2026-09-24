package install_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func scriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "install.sh")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(scriptPath(t))
}

func baseEnv() []string {
	var env []string
	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")
		switch key {
		case "GENGUARD_OS", "GENGUARD_ARCH", "GENGUARD_TAG", "GENGUARD_INSTALL_ROOT", "BINDIR":
			continue
		default:
			env = append(env, e)
		}
	}
	return env
}

func runScript(t *testing.T, args []string, extra []string) (string, int) {
	t.Helper()
	return runScriptEnv(t, args, append(baseEnv(), extra...))
}

func runScriptEnv(t *testing.T, args []string, env []string) (string, int) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	cmd := exec.Command("sh", append([]string{scriptPath(t)}, args...)...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	exit, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatal(err)
	}
	return string(out), exit.ExitCode()
}

func defaultTag(t *testing.T) string {
	t.Helper()
	script, err := os.ReadFile(scriptPath(t))
	if err != nil {
		t.Fatal(err)
	}
	const marker = "default_tag="
	idx := strings.Index(string(script), marker)
	if idx < 0 {
		t.Fatal("default_tag not found")
	}
	rest := string(script)[idx+len(marker):]
	tag, _, _ := strings.Cut(rest, "\n")
	tag = strings.TrimSpace(tag)
	if tag == "" {
		t.Fatal("empty default_tag")
	}
	return tag
}

func TestPrintAsset(t *testing.T) {
	cases := []struct {
		os, arch, tag, want string
	}{
		{"linux", "amd64", "", "genguard_0.5.0_linux_amd64.tar.gz"},
		{"linux", "arm64", "", "genguard_0.5.0_linux_arm64.tar.gz"},
		{"darwin", "amd64", "", "genguard_0.5.0_darwin_amd64.tar.gz"},
		{"darwin", "arm64", "", "genguard_0.5.0_darwin_arm64.tar.gz"},
		{"windows", "amd64", "", "genguard_0.5.0_windows_amd64.zip"},
		{"linux", "amd64", "v1.2.3", "genguard_1.2.3_linux_amd64.tar.gz"},
		{"linux", "amd64", "v1.2.3-rc.1", "genguard_1.2.3-rc.1_linux_amd64.tar.gz"},
	}
	for _, tc := range cases {
		t.Run(tc.os+"/"+tc.arch+"/"+tc.tag, func(t *testing.T) {
			args := []string{"--print-asset"}
			if tc.tag != "" {
				args = append(args, tc.tag)
			}
			out, code := runScript(t, args, []string{
				"GENGUARD_OS=" + tc.os,
				"GENGUARD_ARCH=" + tc.arch,
			})
			if code != 0 {
				t.Fatalf("exit %d: %s", code, out)
			}
			if got := strings.TrimSpace(out); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrintAssetRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  []string
	}{
		{"tag", []string{"--print-asset", "v0.2.0;rm"}, []string{"GENGUARD_OS=linux", "GENGUARD_ARCH=amd64"}},
		{"arch", []string{"--print-asset"}, []string{"GENGUARD_OS=windows", "GENGUARD_ARCH=arm64"}},
		{"os", []string{"--print-asset"}, []string{"GENGUARD_OS=freebsd", "GENGUARD_ARCH=amd64"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runScript(t, tc.args, tc.env)
			if code == 0 {
				t.Fatalf("expected failure, got %q", out)
			}
		})
	}
}

func TestDocsPinMatchesDefaultTag(t *testing.T) {
	root := repoRoot(t)
	tag := defaultTag(t)
	goInstall := "go install github.com/wuddleko/genguard/cmd/genguard@" + tag
	rawURL := regexp.MustCompile(`https://raw\.githubusercontent\.com/wuddleko/genguard/([^/\s]+)/install\.sh`)
	for _, rel := range []string{"README.md", "docs/ci.md"} {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		if strings.Contains(text, "/tmp/genguard-install.sh") {
			t.Fatalf("%s downloads install.sh to a fixed /tmp path", rel)
		}
		if !strings.Contains(text, goInstall) {
			t.Fatalf("%s does not pin %s", rel, goInstall)
		}
		if strings.Contains(text, "v0.2.0") {
			t.Fatalf("%s still pins v0.2.0", rel)
		}
		for _, m := range rawURL.FindAllStringSubmatch(text, -1) {
			pin := m[1]
			if pin != tag {
				t.Fatalf("%s install.sh URL pins %s, default_tag is %s", rel, pin, tag)
			}
			if tagExists(t, pin) && !tagHasFile(t, pin, "install.sh") {
				t.Fatalf("%s pins %s, which does not contain install.sh", rel, pin)
			}
		}
	}
	script, err := os.ReadFile(scriptPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(script), "v0.2.0") {
		t.Fatal("install.sh still pins v0.2.0")
	}
}

func tagExists(t *testing.T, tag string) bool {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/tags/"+tag)
	cmd.Dir = repoRoot(t)
	return cmd.Run() == nil
}

func tagHasFile(t *testing.T, tag, file string) bool {
	t.Helper()
	cmd := exec.Command("git", "cat-file", "-e", tag+":"+file)
	cmd.Dir = repoRoot(t)
	return cmd.Run() == nil
}

func skipNonPOSIXPath(t *testing.T, elems ...string) {
	t.Helper()
	for _, elem := range elems {
		if strings.Contains(elem, ":") {
			t.Skip("path is not a single POSIX PATH entry")
		}
	}
}

func writeExe(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func linkTool(t *testing.T, dir, name string) {
	t.Helper()
	src, err := exec.LookPath(name)
	if err != nil {
		t.Skip(name + " not on PATH")
	}
	if err := os.Symlink(src, filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

func shellBase(t *testing.T, path, home string) []string {
	t.Helper()
	env := []string{
		"PATH=" + path,
		"HOME=" + home,
		"GENGUARD_OS=linux",
		"GENGUARD_ARCH=amd64",
	}
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		env = append(env, "TMPDIR="+tmp)
	}
	return env
}

func TestPrintBindir(t *testing.T) {
	cases := []struct {
		name         string
		sysOnPath    bool
		sysWritable  bool
		brewOnPath   bool
		homeOnPath   bool
		sudoOK       bool
		useBinDir    bool
		binDirOnPath bool
		want         string
	}{
		{name: "sys writable", sysOnPath: true, sysWritable: true, want: "sys"},
		{name: "sys off path uses home", homeOnPath: true, sysWritable: true, want: "home"},
		{name: "brew", brewOnPath: true, want: "brew"},
		{name: "home", homeOnPath: true, want: "home"},
		{name: "sudo", sysOnPath: true, sudoOK: true, want: "sys"},
		{name: "none", want: "fail"},
		{name: "bindir", useBinDir: true, binDirOnPath: true, want: "bindir"},
		{name: "bindir off path", useBinDir: true, want: "fail-bindir"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			sys := filepath.Join(root, "usr", "local", "bin")
			brew := filepath.Join(root, "opt", "homebrew", "bin")
			bindir := filepath.Join(root, "custom")
			fake := filepath.Join(root, "fakebin")
			skipNonPOSIXPath(t, root, home, sys, brew, bindir, fake)
			if err := os.MkdirAll(home, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(fake, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(sys), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(sys, 0o755); err != nil {
				t.Fatal(err)
			}
			if !tc.sysWritable {
				if err := os.Chmod(sys, 0o555); err != nil {
					t.Fatal(err)
				}
			}
			if !tc.sysWritable && writable(sys) {
				t.Skip("directory mode does not remove write access")
			}
			if err := os.MkdirAll(brew, 0o755); err != nil {
				t.Fatal(err)
			}
			sudo := "#!/bin/sh\nexit 1\n"
			if tc.sudoOK {
				sudo = "#!/bin/sh\nif [ \"$1\" = \"-n\" ]; then shift; fi\nif [ \"$1\" = \"true\" ]; then exit 0; fi\nexec \"$@\"\n"
			}
			writeExe(t, filepath.Join(fake, "sudo"), sudo)
			writeExe(t, filepath.Join(fake, "curl"), "#!/bin/sh\necho 'curl should not run' >&2\nexit 1\n")

			var elems []string
			elems = append(elems, fake)
			if tc.sysOnPath {
				elems = append(elems, sys)
			}
			if tc.brewOnPath {
				elems = append(elems, brew)
			}
			if tc.homeOnPath {
				elems = append(elems, filepath.Join(home, ".local", "bin"))
			}
			if tc.binDirOnPath {
				elems = append(elems, bindir)
			}
			elems = append(elems, "/bin", "/usr/bin")
			env := shellBase(t, strings.Join(elems, ":"), home)
			env = append(env, "GENGUARD_INSTALL_ROOT="+root)
			if tc.useBinDir {
				env = append(env, "BINDIR="+bindir)
			}
			out, code := runScriptEnv(t, []string{"--print-bindir"}, env)
			got := strings.TrimSpace(out)
			switch tc.want {
			case "fail":
				if code == 0 {
					t.Fatalf("expected failure, got %q", out)
				}
				if !strings.Contains(out, "no writable directory on PATH") {
					t.Fatalf("exit %d: %s", code, out)
				}
			case "fail-bindir":
				if code == 0 {
					t.Fatalf("expected failure, got %q", out)
				}
				if !strings.Contains(out, "is not on PATH") {
					t.Fatalf("exit %d: %s", code, out)
				}
				if _, err := os.Stat(filepath.Join(bindir, "genguard")); err == nil {
					t.Fatal("installed despite BINDIR off PATH")
				}
			default:
				if code != 0 {
					t.Fatalf("exit %d: %s", code, out)
				}
				want := map[string]string{
					"sys":    sys,
					"brew":   brew,
					"home":   filepath.Join(home, ".local", "bin"),
					"bindir": bindir,
				}[tc.want]
				if got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
			}
		})
	}
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, "w")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

func writeRelease(t *testing.T, rel string, good bool) string {
	t.Helper()
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar not on PATH")
	}
	stage := t.TempDir()
	body := "#!/bin/sh\necho installed\n"
	if err := os.WriteFile(filepath.Join(stage, "genguard"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	asset := "genguard_9.9.9_linux_amd64.tar.gz"
	archive := filepath.Join(rel, asset)
	cmd := exec.Command("tar", "-czf", archive, "-C", stage, "genguard")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v: %s", err, out)
	}
	sum := sha256.Sum256(mustRead(t, archive))
	hash := hex.EncodeToString(sum[:])
	if !good {
		hash = strings.Repeat("a", 64)
	}
	line := hash + "  " + asset + "\n"
	if err := os.WriteFile(filepath.Join(rel, "checksums.txt"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return asset
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fakeCurl(rel string) string {
	return "#!/bin/sh\n" +
		"dest=\nprev=\nurl=\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$prev\" = \"-o\" ]; then dest=$a; prev=; continue; fi\n" +
		"  case \"$a\" in\n" +
		"    -o) prev=-o ;;\n" +
		"    -*) ;;\n" +
		"    *) url=$a ;;\n" +
		"  esac\ndone\n" +
		"base=$(basename \"$url\")\n" +
		"cp '" + rel + "/'\"$base\" \"$dest\"\n"
}

func TestChecksumMismatchDoesNotInstall(t *testing.T) {
	root := t.TempDir()
	bindir := filepath.Join(root, "bin[1]")
	rel := filepath.Join(root, "rel")
	fake := filepath.Join(root, "fakebin")
	skipNonPOSIXPath(t, root, bindir, rel, fake)
	for _, dir := range []string{bindir, rel, fake} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeRelease(t, rel, false)
	writeExe(t, filepath.Join(fake, "curl"), fakeCurl(rel))
	env := shellBase(t, strings.Join([]string{fake, bindir, os.Getenv("PATH")}, ":"), filepath.Join(root, "home"))
	env = append(env, "BINDIR="+bindir, "GENGUARD_INSTALL_ROOT="+root)
	out, code := runScriptEnv(t, []string{"v9.9.9"}, env)
	if code == 0 {
		t.Fatalf("expected failure, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(bindir, "genguard")); err == nil {
		t.Fatal("installed despite checksum mismatch")
	}
}

func TestChecksumInstallsWhenBindirOnPath(t *testing.T) {
	root := t.TempDir()
	bindir := filepath.Join(root, "bin[1]")
	rel := filepath.Join(root, "rel")
	fake := filepath.Join(root, "fakebin")
	skipNonPOSIXPath(t, root, bindir, rel, fake)
	for _, dir := range []string{bindir, rel, fake} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeRelease(t, rel, true)
	writeExe(t, filepath.Join(fake, "curl"), fakeCurl(rel))
	env := shellBase(t, strings.Join([]string{fake, bindir, os.Getenv("PATH")}, ":"), filepath.Join(root, "home"))
	env = append(env, "BINDIR="+bindir, "GENGUARD_INSTALL_ROOT="+root)
	out, code := runScriptEnv(t, []string{"v9.9.9"}, env)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	dest := filepath.Join(bindir, "genguard")
	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("installed mode %o is not executable", info.Mode())
	}
	if !strings.Contains(string(mustRead(t, dest)), "echo installed") {
		t.Fatalf("installed contents: %s", mustRead(t, dest))
	}
}

func TestMissingChecksumEntry(t *testing.T) {
	root := t.TempDir()
	bindir := filepath.Join(root, "bin")
	rel := filepath.Join(root, "rel")
	fake := filepath.Join(root, "fakebin")
	skipNonPOSIXPath(t, root, bindir, rel, fake)
	for _, dir := range []string{bindir, rel, fake} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeRelease(t, rel, true)
	other := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  other.tar.gz\n"
	if err := os.WriteFile(filepath.Join(rel, "checksums.txt"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	writeExe(t, filepath.Join(fake, "curl"), fakeCurl(rel))
	env := shellBase(t, strings.Join([]string{fake, bindir, os.Getenv("PATH")}, ":"), filepath.Join(root, "home"))
	env = append(env, "BINDIR="+bindir, "GENGUARD_INSTALL_ROOT="+root)
	out, code := runScriptEnv(t, []string{"v9.9.9"}, env)
	if code == 0 {
		t.Fatalf("expected failure, got %q", out)
	}
	if !strings.Contains(out, "no entry") {
		t.Fatalf("exit %d: %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(bindir, "genguard")); err == nil {
		t.Fatal("installed despite missing checksum entry")
	}
}

func TestAwkRequired(t *testing.T) {
	root := t.TempDir()
	bindir := filepath.Join(root, "bin")
	rel := filepath.Join(root, "rel")
	fake := filepath.Join(root, "fakebin")
	skipNonPOSIXPath(t, root, bindir, rel, fake)
	for _, dir := range []string{bindir, rel, fake} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeRelease(t, rel, true)
	writeExe(t, filepath.Join(fake, "curl"), fakeCurl(rel))
	linkTool(t, fake, "basename")
	linkTool(t, fake, "cp")
	linkTool(t, fake, "mktemp")
	linkTool(t, fake, "rm")
	env := shellBase(t, fake, filepath.Join(root, "home"))
	env = append(env, "BINDIR="+bindir, "GENGUARD_INSTALL_ROOT="+root)
	// PATH has no awk, which is checked before a bindir is chosen.
	out, code := runScriptEnv(t, []string{"v9.9.9"}, env)
	if code == 0 {
		t.Fatalf("expected failure, got %q", out)
	}
	if !strings.Contains(out, "awk is required") {
		t.Fatalf("exit %d: %s", code, out)
	}
}
