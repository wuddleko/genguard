package cli

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	flags, code, ok := parseCommandFlags(args, stderr, commandUsage{
		name:     "check",
		all:      "Check every genguard.yaml or genguard.yml under the git repository root",
		isolated: "Check the HEAD copy in a throwaway worktree (do not read or write the current checkout)",
	})
	if !ok {
		return code
	}
	path, useAll, code, ok := resolveConfigPath(stderr, flags)
	if !ok {
		return code
	}
	if useAll {
		return runAll(stdout, stderr, check.CheckAllOptions{Since: flags.since, Isolated: flags.isolated}, flags.asJSON, "Generated files match the generators.", check.CheckAll)
	}

	var result check.ConfigResult
	root := filepath.Dir(path)
	if flags.isolated {
		var err error
		result, err = check.CheckSinceIsolated(path, flags.since)
		if err != nil && len(result.Groups) == 0 {
			return errorExit(stderr, path, err.Error())
		}
	} else {
		cfg, err := config.LoadConfig(path)
		if err != nil {
			return errorExit(stderr, path, err.Error())
		}
		result, err = check.CheckSince(cfg, flags.since)
		if err != nil {
			return errorExit(stderr, path, err.Error())
		}
		root = cfg.Root()
	}
	return finishConfig(stdout, stderr, result, path, root, "Generated files match the generators.", flags.asJSON)
}

func runAll(stdout, stderr io.Writer, opts check.CheckAllOptions, asJSON bool, success string, run func(check.CheckAllOptions) (check.RunResult, error)) int {
	result, err := run(opts)
	if err != nil {
		return errorExit(stderr, "", err.Error())
	}
	if len(result.Configs) == 0 {
		return errorExit(stderr, "", "no genguard.yaml or genguard.yml found under repository root")
	}
	return finishRun(stdout, stderr, result, success, asJSON)
}

func runRun(args []string, stdout, stderr io.Writer) int {
	flags, code, ok := parseCommandFlags(args, stderr, commandUsage{
		name:     "run",
		all:      "Run every genguard.yaml or genguard.yml under the git repository root",
		isolated: "Not valid with run; run writes the checkout",
	})
	if !ok {
		return code
	}
	if flags.isolated {
		return errorExit(stderr, "", "genguard run writes the checkout; --isolated is not valid")
	}
	path, useAll, code, ok := resolveConfigPath(stderr, flags)
	if !ok {
		return code
	}
	if useAll {
		return runAll(stdout, stderr, check.CheckAllOptions{Since: flags.since}, flags.asJSON, "Generated files written.", check.RunAll)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		return errorExit(stderr, path, err.Error())
	}
	result, err := check.RunSince(cfg, flags.since)
	if err != nil {
		return errorExit(stderr, path, err.Error())
	}
	return finishConfig(stdout, stderr, result, path, cfg.Root(), "Generated files written.", flags.asJSON)
}

type commandFlags struct {
	all      bool
	selected string
	since    string
	isolated bool
	asJSON   bool
}

type commandUsage struct {
	name     string
	all      string
	isolated string
}

func parseCommandFlags(args []string, stderr io.Writer, usage commandUsage) (commandFlags, int, bool) {
	fs := flag.NewFlagSet(usage.name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	all := fs.Bool("all", false, usage.all)
	configPath := fs.String("config", "", "Path to genguard.yaml (default: walk parents from cwd)")
	configShort := fs.String("c", "", "Path to genguard.yaml (default: walk parents from cwd)")
	since := fs.String("since", "", "Run a group with inputs when its inputs, outputs, or config file differ from HEAD or from the merge-base of this ref, or a declared output file is missing")
	isolated := fs.Bool("isolated", false, usage.isolated)
	asJSON := fs.Bool("json", false, "Print the result as JSON on stdout")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return commandFlags{}, 0, false
		}
		return commandFlags{}, 2, false
	}
	selected := *configPath
	if selected == "" {
		selected = *configShort
	}
	return commandFlags{
		all:      *all,
		selected: selected,
		since:    *since,
		isolated: *isolated,
		asJSON:   *asJSON,
	}, 0, true
}

func resolveConfigPath(stderr io.Writer, flags commandFlags) (path string, useAll bool, code int, ok bool) {
	if flags.all && flags.selected != "" {
		return "", false, errorExit(stderr, "", "--all and --config are mutually exclusive"), false
	}
	if flags.all {
		return "", true, 0, true
	}
	path = flags.selected
	if path == "" {
		found, err := config.FindConfig("")
		if err != nil {
			return "", false, errorExit(stderr, "", err.Error()), false
		}
		if found == "" {
			return "", false, errorExit(stderr, "", "no genguard.yaml found (pass --config)"), false
		}
		path = found
	}
	return path, false, 0, true
}

func finishConfig(stdout, stderr io.Writer, result check.ConfigResult, path, root, success string, asJSON bool) int {
	if asJSON {
		run, err := check.SingleConfigRun(path, result)
		if err != nil {
			return errorExit(stderr, path, err.Error())
		}
		code := writeJSON(stdout, stderr, run)
		maybeAnnotate(stderr, run)
		return code
	}
	code := result.ExitCode()
	if code == 0 {
		var buf strings.Builder
		fmt.Fprintln(&buf, success)
		if result.Skipped() > 0 {
			for _, line := range result.SummaryLines() {
				fmt.Fprintln(&buf, line)
			}
		}
		writePlain(stdout, buf.String())
	} else {
		withoutWorkflowCommands(stderr, func(w io.Writer) {
			report, err := check.FormatFailureReport(result, root)
			fmt.Fprint(w, report)
			if err != nil {
				fmt.Fprintf(w, "error: %v\n", err)
				code = 2
			}
		})
	}
	if annotationsOn() {
		run, err := check.SingleConfigRun(path, result)
		if err != nil {
			writeError(stderr, path, err.Error())
		} else {
			maybeAnnotate(stderr, run)
		}
	}
	return code
}

func finishRun(stdout, stderr io.Writer, run check.RunResult, success string, asJSON bool) int {
	var code int
	if asJSON {
		code = writeJSON(stdout, stderr, run)
	} else if run.ExitCode() == 0 {
		var buf strings.Builder
		fmt.Fprintln(&buf, success)
		for _, line := range run.SuccessLines() {
			fmt.Fprintln(&buf, line)
		}
		writePlain(stdout, buf.String())
	} else {
		withoutWorkflowCommands(stderr, func(w io.Writer) {
			report, err := check.FormatRunFailureReport(run)
			fmt.Fprint(w, report)
			if err != nil {
				fmt.Fprintf(w, "error: %v\n", err)
				code = 2
			} else {
				code = run.ExitCode()
			}
		})
	}
	maybeAnnotate(stderr, run)
	return code
}

func errorExit(stderr io.Writer, file, message string) int {
	writeError(stderr, file, message)
	return 2
}

func writeError(stderr io.Writer, file, message string) {
	withoutWorkflowCommands(stderr, func(w io.Writer) {
		fmt.Fprintf(w, "error: %s\n", message)
	})
	if annotationsOn() {
		fmt.Fprint(stderr, check.FormatErrorAnnotation(file, message))
	}
}

func maybeAnnotate(stderr io.Writer, run check.RunResult) {
	if !annotationsOn() {
		return
	}
	if text := check.FormatAnnotations(run); text != "" {
		fmt.Fprint(stderr, text)
	}
}

func annotationsOn() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true" && os.Getenv("GENGUARD_ANNOTATIONS") != "false"
}

func writePlain(w io.Writer, text string) {
	if os.Getenv("GITHUB_ACTIONS") == "true" && lineStartsWorkflowCommand(text) {
		withoutWorkflowCommands(w, func(buf io.Writer) {
			fmt.Fprint(buf, text)
		})
		return
	}
	fmt.Fprint(w, text)
}

func lineStartsWorkflowCommand(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "::") {
			return true
		}
	}
	return false
}

func withoutWorkflowCommands(w io.Writer, write func(io.Writer)) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		write(w)
		return
	}
	var buf strings.Builder
	write(&buf)
	token := workflowCommandToken()
	fmt.Fprintf(w, "::stop-commands::%s\n", token)
	text := buf.String()
	fmt.Fprint(w, text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		fmt.Fprint(w, "\n")
	}
	fmt.Fprintf(w, "::%s::\n", token)
}

func workflowCommandToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("genguard-%d", os.Getpid())
	}
	return hex.EncodeToString(b[:])
}

func writeJSON(stdout, stderr io.Writer, run check.RunResult) int {
	text, err := check.FormatJSON(run)
	if err != nil {
		withoutWorkflowCommands(stderr, func(w io.Writer) {
			fmt.Fprintf(w, "error: %v\n", err)
		})
		return 2
	}
	fmt.Fprint(stdout, text)
	return run.ExitCode()
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `genguard — fail CI when committed generated outputs drift from their generators

Usage:
  genguard check [-c|--config path/to/genguard.yaml] [--since ref] [--isolated] [--json]
  genguard check --all [--since ref] [--isolated] [--json]
  genguard run [-c|--config path/to/genguard.yaml] [--since ref] [--json]
  genguard run --all [--since ref] [--json]
  genguard version

`)
}
