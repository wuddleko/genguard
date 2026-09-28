package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
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
	return runArgs(nil, args, stdout, stderr, actions.Read())
}

func runArgs(ctx context.Context, args []string, stdout, stderr io.Writer, env actions.Env) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "check":
		return runCheck(ctx, args[1:], stdout, stderr, env)
	case "run":
		return runRun(ctx, args[1:], stdout, stderr, env)
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

func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer, env actions.Env) int {
	flags, code, ok := parseCommandFlags(args, stderr, commandUsage{
		name:     "check",
		all:      "Check every genguard.yaml or genguard.yml under the git repository root",
		isolated: "Check the HEAD copy in a throwaway worktree (do not read or write the current checkout)",
	}, env)
	if !ok {
		return code
	}
	path, useAll, code, ok := resolveConfigPath(stderr, flags, env)
	if !ok {
		return code
	}
	log, quiet := commandWriter(stderr, flags.verbose, env)
	if useAll {
		return runAll(stdout, stderr, check.CheckAllOptions{Since: flags.since, Isolated: flags.isolated, Log: log, Quiet: quiet, Context: ctx, Env: env}, flags.asJSON, "Generated files match the generators.", check.CheckAll, env)
	}

	var result check.ConfigResult
	root := filepath.Dir(path)
	if flags.isolated {
		var err error
		result, err = check.CheckSinceIsolatedLog(ctx, path, flags.since, log, quiet, env)
		if err != nil && len(result.Groups) == 0 {
			return errorExit(stderr, path, err.Error(), env)
		}
	} else {
		cfg, err := config.LoadConfig(path)
		if err != nil {
			return errorExit(stderr, path, err.Error(), env)
		}
		result, err = check.CheckSinceLog(ctx, cfg, flags.since, log, quiet, env)
		if err != nil {
			return errorExit(stderr, path, err.Error(), env)
		}
		root = cfg.Root()
	}
	return finishConfig(stdout, stderr, result, path, root, "Generated files match the generators.", flags.asJSON, env)
}

func runAll(stdout, stderr io.Writer, opts check.CheckAllOptions, asJSON bool, success string, run func(check.CheckAllOptions) (check.RunResult, error), env actions.Env) int {
	result, err := run(opts)
	if err != nil {
		return errorExit(stderr, "", err.Error(), env)
	}
	if len(result.Configs) == 0 {
		return errorExit(stderr, "", "no genguard.yaml or genguard.yml found under repository root", env)
	}
	return finish(stdout, stderr, result, success, false, asJSON, env)
}

func runRun(ctx context.Context, args []string, stdout, stderr io.Writer, env actions.Env) int {
	flags, code, ok := parseCommandFlags(args, stderr, commandUsage{
		name:     "run",
		all:      "Run every genguard.yaml or genguard.yml under the git repository root",
		isolated: "Not valid with run; run writes the checkout",
	}, env)
	if !ok {
		return code
	}
	if flags.isolated {
		return errorExit(stderr, "", "genguard run writes the checkout; --isolated is not valid", env)
	}
	path, useAll, code, ok := resolveConfigPath(stderr, flags, env)
	if !ok {
		return code
	}
	log, quiet := commandWriter(stderr, flags.verbose, env)
	if useAll {
		return runAll(stdout, stderr, check.CheckAllOptions{Since: flags.since, Log: log, Quiet: quiet, Context: ctx, Env: env}, flags.asJSON, "Generated files written.", check.RunAll, env)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		return errorExit(stderr, path, err.Error(), env)
	}
	result, err := check.RunSinceLog(ctx, cfg, flags.since, log, quiet, env)
	if err != nil {
		return errorExit(stderr, path, err.Error(), env)
	}
	return finishConfig(stdout, stderr, result, path, cfg.Root(), "Generated files written.", flags.asJSON, env)
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

func resolveConfigPath(stderr io.Writer, flags commandFlags, env actions.Env) (path string, useAll bool, code int, ok bool) {
	if flags.all && flags.selected != "" {
		return "", false, errorExit(stderr, "", "--all and --config are mutually exclusive", env), false
	}
	if flags.all {
		return "", true, 0, true
	}
	path = flags.selected
	if path == "" {
		root, err := check.RepoRoot("")
		if err != nil {
			return "", false, errorExit(stderr, "", err.Error(), env), false
		}
		found, err := config.FindConfig("", root)
		if err != nil {
			return "", false, errorExit(stderr, "", err.Error(), env), false
		}
		if found == "" {
			return "", false, errorExit(stderr, "", "no genguard.yaml or genguard.yml found (pass --config)", env), false
		}
		path = found
	}
	return path, false, 0, true
}

func finishConfig(stdout, stderr io.Writer, result check.ConfigResult, path, root, success string, asJSON bool, env actions.Env) int {
	run, err := check.SingleConfigRun(path, result)
	if err != nil {
		if asJSON {
			return errorExit(stderr, path, err.Error(), env)
		}
		code := renderFinish(stdout, stderr, check.RunResult{
			Configs: []check.ConfigRun{{
				Path:   filepath.Join(root, "genguard.yaml"),
				Result: result,
			}},
		}, success, true, false, env)
		if env.Annotating() {
			writeError(stderr, path, err.Error(), env)
		}
		return code
	}
	return finish(stdout, stderr, run, success, true, asJSON, env)
}

func finish(stdout, stderr io.Writer, run check.RunResult, success string, singleConfig, asJSON bool, env actions.Env) int {
	code := renderFinish(stdout, stderr, run, success, singleConfig, asJSON, env)
	maybeAnnotate(stderr, run, env)
	return code
}

func renderFinish(stdout, stderr io.Writer, run check.RunResult, success string, singleConfig, asJSON bool, env actions.Env) int {
	var code int
	if asJSON {
		writeCommandTail(stderr, commandTails(run, singleConfig), env)
		code = writeJSON(stdout, stderr, run, env)
	} else if run.ExitCode() == 0 {
		var buf strings.Builder
		fmt.Fprintln(&buf, success)
		for _, line := range successLines(run, singleConfig) {
			fmt.Fprintln(&buf, line)
		}
		writePlain(stdout, buf.String(), env)
	} else {
		withoutWorkflowCommands(stderr, env, func(w io.Writer) {
			report, err := failureReport(run, singleConfig)
			fmt.Fprint(w, report)
			if err != nil {
				fmt.Fprintf(w, "error: %v\n", err)
				code = 2
			} else {
				code = run.ExitCode()
			}
		})
	}
	return code
}

func successLines(run check.RunResult, singleConfig bool) []string {
	if !singleConfig {
		return run.SuccessLines()
	}
	if len(run.Configs) != 1 || run.Configs[0].Err != nil || run.Configs[0].Result.Skipped() == 0 {
		return nil
	}
	return run.Configs[0].Result.SummaryLines()
}

func commandTails(run check.RunResult, singleConfig bool) string {
	if singleConfig && len(run.Configs) == 1 && run.Configs[0].Err == nil {
		return check.FormatCommandTails(run.Configs[0].Result)
	}
	if singleConfig {
		return ""
	}
	return check.FormatRunCommandTails(run)
}

func failureReport(run check.RunResult, singleConfig bool) (string, error) {
	if !singleConfig {
		return check.FormatRunFailureReport(run)
	}
	if len(run.Configs) != 1 || run.Configs[0].Err != nil {
		return "", nil
	}
	return check.FormatFailureReport(run.Configs[0].Result, filepath.Dir(run.Configs[0].Path))
}

func writeCommandTail(stderr io.Writer, tails string, env actions.Env) {
	if tails == "" {
		return
	}
	withoutWorkflowCommands(stderr, env, func(w io.Writer) {
		fmt.Fprint(w, tails)
	})
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
	token, err := actions.Token()
	if err != nil {
		token = fmt.Sprintf("genguard-%d", os.Getpid())
	}
	bracket := actions.Bracket{W: w, Token: token}
	bracket.Open()
	text := buf.String()
	fmt.Fprint(w, text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		fmt.Fprint(w, "\n")
	}
	bracket.Close()
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
