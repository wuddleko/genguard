package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/config"
)

// Version is shown by `genguard version` and `--version`. Releases overwrite it
// via main.version ldflags; local builds keep "dev".
var Version = "dev"

func Run(args []string) int {
	return RunWithIO(args, os.Stdout, os.Stderr)
}

func RunWithIO(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, Version)
		return 0
	case "-h", "--help", "help":
		printUsage(stderr)
		return 0
	default:
		printUsage(stderr)
		return 2
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	all := fs.Bool("all", false, "Check every genguard.yaml or genguard.yml under the git repository root")
	configPath := fs.String("config", "", "Path to genguard.yaml (default: walk parents from cwd)")
	configShort := fs.String("c", "", "Path to genguard.yaml (default: walk parents from cwd)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	selected := *configPath
	if selected == "" {
		selected = *configShort
	}
	if *all && selected != "" {
		fmt.Fprint(stderr, "error: --all and --config are mutually exclusive\n")
		return 2
	}
	if *all {
		return runCheckAll(stdout, stderr)
	}

	path := selected
	if path == "" {
		found, err := config.FindConfig("")
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		if found == "" {
			fmt.Fprint(stderr, "error: no genguard.yaml found (pass --config)\n")
			return 2
		}
		path = found
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}

	result, err := check.CheckConfig(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if result.ExitCode() == 0 {
		fmt.Fprintln(stdout, "Generated files match the generators.")
		return 0
	}

	report, err := check.FormatFailureReport(result, cfg.Root())
	fmt.Fprint(stderr, report)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	return result.ExitCode()
}

func runCheckAll(stdout, stderr io.Writer) int {
	run, err := check.CheckAll(check.CheckAllOptions{})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if len(run.Configs) == 0 {
		fmt.Fprint(stderr, "error: no genguard.yaml or genguard.yml found under repository root\n")
		return 2
	}
	if run.ExitCode() == 0 {
		fmt.Fprintln(stdout, "Generated files match the generators.")
		for _, line := range run.SuccessLines() {
			fmt.Fprintln(stdout, line)
		}
		return 0
	}

	report, err := check.FormatRunFailureReport(run)
	fmt.Fprint(stderr, report)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	return run.ExitCode()
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `genguard — fail CI when committed generated outputs drift from their generators

Usage:
  genguard check [-c|--config path/to/genguard.yaml]
  genguard check --all
  genguard version

`)
}
