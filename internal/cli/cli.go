package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/config"
)

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
	case "run":
		return runRun(args[1:], stdout, stderr)
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
	since := fs.String("since", "", "Run a group with inputs when its inputs, outputs, or config file differ from HEAD or from the merge-base of this ref, or a declared output file is missing")
	isolated := fs.Bool("isolated", false, "Check the HEAD copy in a throwaway worktree (do not read or write the current checkout)")
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
		return runCheckAll(stdout, stderr, *since, *isolated)
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

	var result check.ConfigResult
	root := filepath.Dir(path)
	if *isolated {
		var err error
		result, err = check.CheckSinceIsolated(path, *since)
		if err != nil && len(result.Groups) == 0 {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
	} else {
		cfg, err := config.LoadConfig(path)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		result, err = check.CheckSince(cfg, *since)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 2
		}
		root = cfg.Root()
	}
	if result.ExitCode() == 0 {
		fmt.Fprintln(stdout, "Generated files match the generators.")
		if result.Skipped() > 0 {
			for _, line := range result.SummaryLines() {
				fmt.Fprintln(stdout, line)
			}
		}
		return 0
	}

	report, err := check.FormatFailureReport(result, root)
	fmt.Fprint(stderr, report)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	return result.ExitCode()
}

func runCheckAll(stdout, stderr io.Writer, since string, isolated bool) int {
	run, err := check.CheckAll(check.CheckAllOptions{Since: since, Isolated: isolated})
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

func runRun(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	configPath := fs.String("config", "", "Path to genguard.yaml (default: walk parents from cwd)")
	configShort := fs.String("c", "", "Path to genguard.yaml (default: walk parents from cwd)")
	since := fs.String("since", "", "Run a group with inputs when its inputs, outputs, or config file differ from HEAD or from the merge-base of this ref, or a declared output file is missing")
	isolated := fs.Bool("isolated", false, "Not valid with run; run writes the checkout")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *isolated {
		fmt.Fprint(stderr, "error: genguard run writes the checkout; --isolated is not valid\n")
		return 2
	}

	selected := *configPath
	if selected == "" {
		selected = *configShort
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
	result, err := check.RunSince(cfg, *since)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 2
	}
	if result.ExitCode() == 0 {
		fmt.Fprintln(stdout, "Generated files written.")
		if result.Skipped() > 0 {
			for _, line := range result.SummaryLines() {
				fmt.Fprintln(stdout, line)
			}
		}
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

func printUsage(w io.Writer) {
	fmt.Fprint(w, `genguard — fail CI when committed generated outputs drift from their generators

Usage:
  genguard check [-c|--config path/to/genguard.yaml] [--since ref] [--isolated]
  genguard check --all [--since ref] [--isolated]
  genguard run [-c|--config path/to/genguard.yaml] [--since ref]
  genguard version

`)
}
