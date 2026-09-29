package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/wuddleko/genguard/internal/actions"
	"github.com/wuddleko/genguard/internal/check"
	"github.com/wuddleko/genguard/internal/config"
)

var Version = "dev"

func Run(args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// stop restores the default action so a second signal kills the process.
	go func() {
		<-ctx.Done()
		stop()
	}()
	return runArgs(ctx, args, os.Stdout, os.Stderr, actions.Read())
}

func RunWithIO(args []string, stdout, stderr io.Writer) int {
	return runArgs(context.Background(), args, stdout, stderr, actions.Read())
}

func runArgs(ctx context.Context, args []string, stdout, stderr io.Writer, env actions.Env) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "check":
		return runCommand(ctx, check.ModeCheck, args[1:], stdout, stderr, env)
	case "run":
		return runCommand(ctx, check.ModeRun, args[1:], stdout, stderr, env)
	case "version", "--version":
		fmt.Fprintln(stdout, Version)
		return 0
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

var commandUsages = map[check.Mode]commandUsage{
	check.ModeCheck: {
		name:     "check",
		all:      "Check every genguard.yaml or genguard.yml under the git repository root",
		isolated: "Check the HEAD copy in a throwaway worktree (do not read or write the current checkout)",
		success:  "Generated files match the generators.",
	},
	check.ModeRun: {
		name:     "run",
		all:      "Run every genguard.yaml or genguard.yml under the git repository root",
		isolated: "Not valid with run; run writes the checkout",
		success:  "Generated files written.",
	},
}

func runCommand(ctx context.Context, mode check.Mode, args []string, stdout, stderr io.Writer, env actions.Env) int {
	usage := commandUsages[mode]
	flags, code, ok := parseCommandFlags(args, stderr, usage, env)
	if !ok {
		return code
	}
	if mode == check.ModeRun && flags.isolated {
		return errorExit(stderr, "", "genguard run writes the checkout; --isolated is not valid", env)
	}
	path, repoRoot, useAll, code, ok := resolveConfigPath(stderr, flags, env)
	if !ok {
		return code
	}
	log, quiet := commandWriter(stderr, flags.verbose, env)
	opts := check.Options{Mode: mode, Context: ctx, Since: flags.since, Isolated: flags.isolated, Log: log, Quiet: quiet, Env: env, RepoRoot: repoRoot}

	var run check.RunResult
	var err error
	if useAll {
		run, err = check.ExecuteAll(opts)
		if err == nil && len(run.Configs) == 0 {
			err = errors.New("no genguard.yaml or genguard.yml found under repository root")
		}
	} else {
		run, err = check.Execute(opts, path)
	}
	if err != nil {
		return errorExit(stderr, path, err.Error(), env)
	}
	code = render(stdout, stderr, run, usage.success, flags.asJSON, env)
	maybeAnnotate(stderr, run, env)
	return code
}

type commandFlags struct {
	all      bool
	selected string
	since    string
	isolated bool
	asJSON   bool
	verbose  bool
}

type commandUsage struct {
	name     string
	all      string
	isolated string
	success  string
}

func parseCommandFlags(args []string, stderr io.Writer, usage commandUsage, env actions.Env) (commandFlags, int, bool) {
	fs := flag.NewFlagSet(usage.name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	all := fs.Bool("all", false, usage.all)
	configPath := fs.String("config", "", "Path to genguard.yaml (default: walk from cwd up to the git repository root)")
	configShort := fs.String("c", "", "Path to genguard.yaml (default: walk from cwd up to the git repository root)")
	since := fs.String("since", "", "Run a group with inputs when its inputs, outputs, or config file differ from HEAD or from the merge-base of this ref, or a declared output file is missing")
	isolated := fs.Bool("isolated", false, usage.isolated)
	asJSON := fs.Bool("json", false, "Print the result as JSON on stdout")
	verbose := fs.Bool("verbose", false, "Stream generator output to stderr")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return commandFlags{}, 0, false
		}
		return commandFlags{}, 2, false
	}
	if *configPath != "" && *configShort != "" && *configPath != *configShort {
		return commandFlags{}, errorExit(stderr, "", "cannot use -c and --config with different paths", env), false
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
		verbose:  *verbose,
	}, 0, true
}

func commandWriter(stderr io.Writer, verbose bool, env actions.Env) (io.Writer, bool) {
	if !verbose && !env.Actions {
		return nil, false
	}
	return stderr, !verbose
}

// resolveConfigPath returns the repository root too when it found the config
// by walking up from the working directory.
func resolveConfigPath(stderr io.Writer, flags commandFlags, env actions.Env) (path, repoRoot string, useAll bool, code int, ok bool) {
	if flags.all && flags.selected != "" {
		return "", "", false, errorExit(stderr, "", "--all and --config are mutually exclusive", env), false
	}
	if flags.all {
		return "", "", true, 0, true
	}
	if flags.selected != "" {
		return flags.selected, "", false, 0, true
	}
	root, err := check.RepoRoot("")
	if err != nil {
		return "", "", false, errorExit(stderr, "", err.Error(), env), false
	}
	found, err := config.FindConfig("", root)
	if err != nil {
		return "", "", false, errorExit(stderr, "", err.Error(), env), false
	}
	if found == "" {
		return "", "", false, errorExit(stderr, "", "no genguard.yaml or genguard.yml found (pass --config)", env), false
	}
	return found, root, false, 0, true
}

func render(stdout, stderr io.Writer, run check.RunResult, success string, asJSON bool, env actions.Env) int {
	if asJSON {
		if tails := check.FormatCommandTails(run); tails != "" {
			withoutWorkflowCommands(stderr, env, func(w io.Writer) {
				fmt.Fprint(w, tails)
			})
		}
		return writeJSON(stdout, stderr, run, env)
	}
	code := run.ExitCode()
	if code == 0 {
		var buf strings.Builder
		fmt.Fprintln(&buf, success)
		for _, line := range run.SuccessLines() {
			fmt.Fprintln(&buf, line)
		}
		writePlain(stdout, buf.String(), env)
		return 0
	}
	withoutWorkflowCommands(stderr, env, func(w io.Writer) {
		report, err := check.FormatFailureReport(run)
		fmt.Fprint(w, report)
		if err != nil {
			fmt.Fprintf(w, "error: %v\n", err)
			code = 2
		}
	})
	return code
}

func errorExit(stderr io.Writer, file, message string, env actions.Env) int {
	writeError(stderr, file, message, env)
	return 2
}

func writeError(stderr io.Writer, file, message string, env actions.Env) {
	withoutWorkflowCommands(stderr, env, func(w io.Writer) {
		fmt.Fprintf(w, "error: %s\n", message)
	})
	if env.Annotating() {
		fmt.Fprint(stderr, check.FormatErrorAnnotation(file, message, env))
	}
}

func maybeAnnotate(stderr io.Writer, run check.RunResult, env actions.Env) {
	if !env.Annotating() {
		return
	}
	if text := check.FormatAnnotations(run, env); text != "" {
		fmt.Fprint(stderr, text)
	}
}

func writePlain(w io.Writer, text string, env actions.Env) {
	if env.Actions && lineStartsWorkflowCommand(text) {
		withoutWorkflowCommands(w, env, func(buf io.Writer) {
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

func withoutWorkflowCommands(w io.Writer, env actions.Env, write func(io.Writer)) {
	if !env.Actions {
		write(w)
		return
	}
	var buf strings.Builder
	write(&buf)
	actions.WriteBracketed(w, buf.String())
}

func writeJSON(stdout, stderr io.Writer, run check.RunResult, env actions.Env) int {
	text, err := check.FormatJSON(run)
	if err != nil {
		withoutWorkflowCommands(stderr, env, func(w io.Writer) {
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
  genguard check [-c|--config path/to/genguard.yaml] [--since ref] [--isolated] [--json] [--verbose]
  genguard check --all [--since ref] [--isolated] [--json] [--verbose]
  genguard run [-c|--config path/to/genguard.yaml] [--since ref] [--json] [--verbose]
  genguard run --all [--since ref] [--json] [--verbose]
  genguard version

`)
}
