package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wuddleko/genguard/internal/config"
	"github.com/wuddleko/genguard/tests/testutil"
)

func TestCheckAndRunCleanRefuseLoadedConfig(t *testing.T) {
	for _, relative := range []bool{false, true} {
		for _, run := range []bool{false, true} {
			name := "check"
			if run {
				name = "run"
			}
			if relative {
				name += "-relative"
			}
			t.Run(name, func(t *testing.T) {
				parent := t.TempDir()
				root := filepath.Join(parent, "repo")
				if err := testutil.InitGitRepo(root); err != nil {
					t.Fatal(err)
				}
				path, err := testutil.WriteGenguardConfig(root, "", "", "custom.yaml", []testutil.GroupSpec{{
					Name:    "gen",
					Command: `python3 -c "open('ran','w').close()"`,
					Outputs: []string{"custom.yaml"},
					Clean:   true,
				}})
				if err != nil {
					t.Fatal(err)
				}
				loadPath := path
				if relative {
					chdir(t, parent)
					loadPath = filepath.Join("repo", "custom.yaml")
				}
				cfg, err := config.LoadConfig(loadPath)
				if err != nil {
					t.Fatal(err)
				}
				if !filepath.IsAbs(cfg.Path) {
					t.Fatalf("config path = %q", cfg.Path)
				}
				loadedInfo, err := os.Stat(cfg.Path)
				if err != nil {
					t.Fatal(err)
				}
				wantInfo, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				if !os.SameFile(loadedInfo, wantInfo) {
					t.Fatalf("config path = %q", cfg.Path)
				}
				var result ConfigResult
				if run {
					result, err = RunConfig(cfg)
				} else {
					result, err = CheckConfig(cfg)
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Groups) != 1 || result.Groups[0].Status != GroupError || result.Groups[0].Err == nil || !strings.Contains(result.Groups[0].Err.Error(), "config file") {
					t.Fatalf("result = %+v", result.Groups)
				}
				if _, statErr := os.Lstat(path); statErr != nil {
					t.Fatalf("config removed: %v", statErr)
				}
				if _, statErr := os.Lstat(filepath.Join(root, "ran")); !os.IsNotExist(statErr) {
					t.Fatalf("command ran: %v", statErr)
				}
			})
		}
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Error(err)
		}
	})
}
