# genguard

[![CI](https://github.com/wuddleko/genguard/actions/workflows/ci.yml/badge.svg)](https://github.com/wuddleko/genguard/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/wuddleko/genguard.svg)](https://pkg.go.dev/github.com/wuddleko/genguard)
[![Release](https://img.shields.io/github/v/release/wuddleko/genguard)](https://github.com/wuddleko/genguard/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> Detect generated-code drift in CI by re-running your existing generators.

genguard is a Git-aware code generation checker. It re-runs the commands in `genguard.yaml` and fails when a fresh run does not match the generated files committed in Git.

Any generator you can start with a command works, including:

- Go, with `go generate`
- Protobuf, with `buf generate`
- SQL, with `sqlc generate`
- OpenAPI client and type generators
- a Makefile target, or any other command

The generators stay yours. genguard does not install them, and it does not commit the result. `check` leaves the new files in your working tree. `--isolated` runs the same check in a temporary worktree and leaves your checkout alone.

CI runs the same `genguard check`. GitHub Actions snippets are in [docs/ci.md](docs/ci.md).

## Compared with `git diff`

Generating and then running `git diff` can show this mismatch. You still have to remember the output paths, compare to `HEAD` rather than the index (a staged file looks clean otherwise), and remove the old files first so a file the generator stopped writing is not left behind. A second generator means doing that again in a script.

genguard keeps those rules next to the generated files and only looks at the paths you list:

- Compares those outputs to `HEAD`. A file that is only staged still fails until it is committed.
- Can delete the declared outputs before the command, when `clean` is set.
- Runs several generator groups, in order, from one file.
- Skips an unchanged group with `--since`.
- Can check the committed files in a temporary worktree with `--isolated`.

```yaml
clean: true # delete declared outputs before the command, so a file the generator stopped writing is not left behind
groups:
  - name: protobuf
    command: buf generate
    inputs:
      - proto/
    outputs:
      - gen/
  - name: sqlc
    command: sqlc generate
    inputs:
      - queries/
    outputs:
      - internal/db/
    clean: false # overrides the clean above
```

Put `genguard.yaml` (or `genguard.yml`) next to the generated files, outside any directory listed in `outputs`. Templates for the usual tools are in [examples/](examples/README.md). Those files are config only. The generator stays yours.

## Quick start

With Go 1.22+:

```bash
go install github.com/wuddleko/genguard/cmd/genguard@v0.6.0
genguard check
```

`$(go env GOPATH)/bin` has to be on your `PATH`. A match prints `Generated files match the generators.` and exits 0. Drift prints a summary, the paths, and a diff, then exits 1. [Install](#install) covers a machine without Go. [Run it](#run-it) lists every exit code.

## Use cases

### Generated Go code

Point `command` at `go generate ./...` and list the directories it writes. The generated files have to be committed. genguard fails when a fresh run would change them. [Template](examples/go-generate.yaml).

### Protobuf

Run `buf generate` and list the generated tree under `outputs`. Add `proto/` under `inputs` when `--since` should skip the group if those files are untouched. [Template](examples/buf.yaml).

### SQL with sqlc

`sqlc generate` checks the generated database package against `HEAD`. [Template](examples/sqlc.yaml).

### OpenAPI

Point your OpenAPI generator at the spec and list the client or types it writes. The template uses `openapi-generator-cli`. [Template](examples/openapi.yaml).

### More than one config

In a monorepo, keep a `genguard.yaml` beside each service. `genguard check --all` runs every config it finds.

## Incremental checks

`genguard check --since origin/main` skips a group that declares `inputs` when those paths, its outputs, and the config still match the latest commit that `HEAD` and that ref share. It still runs if a listed file output is absent, if `git diff` against that merge-base or against `HEAD` names any of those paths, or if any of those specs has an untracked file.

A group with no `inputs` still runs. A skipped group does not run its command, and `clean` does not delete its outputs. A pull request that touches one generator can leave the expensive ones alone.

## Commands

- `check` — run the generators, leave their files in place, and fail if the declared outputs differ from `HEAD`. A file that is only staged still fails until it is committed.
- `run` — run the generators and leave their files in place. A success exits 0.
- `version` — print the version.
- `-c`, `--config` — config file to use. Otherwise genguard walks up from the current directory for `genguard.yaml` or `genguard.yml`, inside a git work tree.
- `--all` — every config under the repository. `check --all` lists the index plus untracked files that are not ignored (`git ls-files`). `--all --isolated` lists HEAD (`git ls-tree`): a staged-only config is absent, and a committed config removed from the index is still listed. A gitignored untracked config is in neither list. A config under `.git`, `vendor`, or `node_modules` is skipped. If git cannot open a directory that can hold a config, discovery fails.
- `--since` — skip a group with `inputs` when those paths, its outputs, and the config still match the latest commit that `HEAD` and the ref share. It still runs if a listed file output is absent, if that merge-base or `HEAD` differs, or if any of those specs has an untracked file.
- `--isolated` — check the committed files in a temporary worktree and leave your checkout alone. `check` only.
- `--json` — print the result as JSON. The document has `exit` and `configs`. Each config has `path`, `exit`, and `groups`; each group has `name`, `status`, `error`, `drifts` (`kind`, `path`), and `tools` (`want`, `have`). A run that stops before any config runs still prints `{"exit": 2, "error": "...", "configs": []}` on stdout. The `error:` line stays on stderr. A flag the parser does not know prints usage only.
- `--verbose` — stream the generator's stdout and stderr to stderr while the command runs.

`check` and `run` take flags only. A positional argument exits 2 with `unexpected argument`. Pass a config with `--config`. Before, a leftover positional was ignored, and so was every flag after it: `genguard check svc/genguard.yaml --json` checked the config found from the working directory and printed text.

## Run it

```bash
genguard check
```

A match prints `Generated files match the generators.` and exits 0. Drift prints a summary, the paths, and a diff, then exits 1. Regenerate and commit. `run` prints `Generated files written.` on success.

| Exit | Meaning |
|---|---|
| `0` | `check`: outputs match `HEAD`, including when some groups were skipped. `run`: the commands succeeded |
| `1` | Drift (`check` only) |
| `2` | Config, git, or the generator command failed. A command error wins over drift |

A command that exits 0 stays quiet, including when the check finds drift. A failed command prints its last 50 lines on stderr, labeled with the group name, then the Summary. With `--all` the label is the config path, a colon, a space, and the group name (`services/api/genguard.yaml: greeting:`). The summary line names `command failed (exit N)`. When that is the only failure, the final line is `error: command failed (exit N)`. A command that prints nothing uses `command failed (exit N): no output`. `--json` keeps the document on stdout. Those lines stay on stderr, and the group's `error` field is that same sentence.

`--verbose` prints each line as it arrives, with the same label on the first line. It applies to `check` and `run`, including `--all`, `--since`, `--isolated`, and `--json`. The Summary names the exit and leaves the lines where they were printed.

An interrupt before any group has run prints `error: interrupted` and exits 2. After that, the Summary covers the groups that ran and the final line adds `error: interrupted`, including when the earlier groups all passed or one had already failed.

## Install

With Go 1.22+:

```bash
go install github.com/wuddleko/genguard/cmd/genguard@v0.6.0
```

`$(go env GOPATH)/bin` has to be on your `PATH`. From a checkout of this repository: `make install` or `go install ./cmd/genguard`.

Without Go, [install.sh](install.sh) downloads the release for this machine, checks the archive against `checksums.txt`, and installs into a directory already on `PATH`. From a checkout: `sh install.sh`. Pass a tag, or set `GENGUARD_TAG`, for another release. `sh install.sh --help` lists location and platform options.

## Config

With no `--config`, genguard walks up from the current directory for `genguard.yaml` or `genguard.yml`, inside a git work tree. A directory holds one of those names. Both files, or `--all` together with `--config`, exit 2.

Top-level keys are `groups` (required, non-empty), `clean`, `tools`, and `timeout`. An unknown key is an error.

- **`name`** — optional label. The fallback is `groups[0]`, `groups[1]`, and so on. A name that is null, empty, or whitespace uses that fallback.
- **`command`** — required. Run with `sh -c` from the config directory. On Windows, `sh` or `bash` on `PATH`; otherwise `%COMSPEC% /C`.
- **`outputs`** — required, non-empty literal paths and globs, relative to the config file. An entry starting with `:` is a config error. A null or blank entry must be a non-empty string.
- **`inputs`** — optional paths for `--since`, with the same rules as `outputs`. Omit the key and the group runs every time. An empty list is an error. A null or blank entry must be a non-empty string.
- **`clean`** — optional, default `false`. Delete the outputs before the command. A top-level `clean` applies to every group; a group can override it. `clean: ~` is a boolean error. Omitting the key leaves it unset.
- **`tools`** — optional names from the top-level `tools` list.
- **`timeout`** — optional Go duration greater than zero (`200ms`, `1m`). A top-level value applies to every group. A group value replaces it. Omitting it on a group keeps the top-level value. Probes use that same limit. When the key is absent, the command has no limit. `0s`, a negative duration, a non-string, and a string that is not a Go duration are errors. `timeout: ~` is a duration error. Omitting the key leaves it unset. A timeout is a group error (`command timed out after 200ms`), exit 2. The tree is still diffed, and a `clean` wipe is left as it is.

A missing key stays the default. A present null and a present empty value take the same branch.

### Tools

`tools` declares programs a group can require. Each entry has:

- **`name`** — required, a single token (`git`, `sqlc`).
- **`version`** — optional exact version. A leading `v` is stripped from the pin and from the first version-shaped token in the probe output, and those two texts have to match. `v1.32.0` and `1.32.0` are the same pin. A `version` or `command` that is null, `""`, or whitespace requires a non-empty string. Omitting the key leaves it unset.
- **`command`** — optional probe. The default is `name --version`.

A group lists those names under its own `tools` key. A name that is not declared at the top level is a config error. The probes run before `clean` and before the generator. A missing program, or a version that does not match, fails the group and the command does not run. The groups of one config share a probe with the same command, pin, and timeout. Probes run in the config directory, so configs do not share them. On OK and drift, the Summary appends the found versions in list order: `protobuf: drift (1 modified); buf 1.32.0`. An error line already names the tool. With `--json`, `want` is the pin and `have` is the probed version, both without a leading `v`. When `version` is omitted, `want` is omitted and `have` is the probed version. Omit `tools` when the group was skipped or listed none.

After the command, genguard compares `HEAD` to the working tree under `outputs`:

| What you see | What happened |
|---|---|
| `modified` | A tracked file changed |
| `untracked` | A new file that is not gitignored |
| `missing` | A tracked file under `outputs` is gone, or a listed **file** is absent. A directory or a glob is not, by itself, a missing path |

Generated paths have to be tracked. Gitignored files under `outputs` stay out of the report. A failed command is still diffed, and that group counts as an error (exit 2). Its last 50 lines are printed above the Summary.

`clean: true` removes the declared outputs before the command: a directory is recreated empty, a file is removed, a glob removes the tracked and untracked matches. A glob that matches a directory is an error. It refuses `.`, `..`, absolute paths, anything outside the config directory, symlinks, and a delete that would take `.git` or the config file. A failed command does not restore the wipe.

Two output specs that can name the same path are a load error. A glob follows git: `*`, `?`, and `[...]` also match `/`, so `gen/*.go` overlaps `gen/sub/`. A trailing slash is the whole directory, and so is a listed directory without one. A literal overlaps a glob only when that literal matches the glob, and two globs overlap only when one path could match both. Within one file the error names both groups and both specs. `check --all`, `run --all`, and `check --all --isolated` use the same check across configs and name both config paths. Isolated reads the committed files and directories, including a config removed from the checkout. A file that does not parse is reported for that config. Outputs from a file that parses still count, so another config that names one of those paths does not run. The commands do not run, so neither path is reported as drift.

Groups run in order and share the tree. `--all --isolated` gives each config its own worktree. `--isolated` is only valid with `check`. A bad `--since` ref exits 2 before any group runs. An unchanged group is `skipped`: its command does not run, and `clean` does not delete its outputs.

## Security

What a run is allowed to do, and how to report a vulnerability: [SECURITY.md](SECURITY.md).

## CI

GitHub Actions snippets and per-tool setup: [docs/ci.md](docs/ci.md).

## Development

```bash
make test
make lint   # go vet ./...
```

Tests need `git` and `python3`.

## License

MIT. See [LICENSE](LICENSE).
