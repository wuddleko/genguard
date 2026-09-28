package check

import (
	"context"
	"io"

	"github.com/wuddleko/genguard/internal/config"
)

func RunConfig(cfg config.Config) (ConfigResult, error) {
	return executeConfig(cfg, Options{Mode: ModeRun})
}

func RunAll(opts CheckAllOptions) (RunResult, error) {
	return executeAll(optionsFrom(opts, ModeRun))
}

func RunSince(cfg config.Config, since string) (ConfigResult, error) {
	return executeConfig(cfg, Options{Mode: ModeRun, Since: since})
}

func RunSinceLog(ctx context.Context, cfg config.Config, since string, log io.Writer, quiet bool) (ConfigResult, error) {
	return executeConfig(cfg, Options{Mode: ModeRun, Since: since, Log: log, Quiet: quiet, Context: ctx})
}
