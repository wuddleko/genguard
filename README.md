# genguard

[![CI](https://github.com/wuddleko/genguard/actions/workflows/ci.yml/badge.svg)](https://github.com/wuddleko/genguard/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/wuddleko/genguard.svg)](https://pkg.go.dev/github.com/wuddleko/genguard)
[![Release](https://img.shields.io/github/v/release/wuddleko/genguard)](https://github.com/wuddleko/genguard/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> Catch generated-code drift in CI by re-running the generators you already use.

**genguard** is a Git-aware checker for generated files. It runs the commands in your `genguard.yaml`, then compares the result to what is committed at `HEAD`. If they differ, the check fails.

Works with anything you can invoke from a shell:

- Go — `go generate`
- Protobuf — `buf generate`
- SQL — `sqlc generate`
- OpenAPI clients and types
- A Makefile target, or any other command

You keep your generators. genguard does not install tools or commit output for you. `check` leaves new files in your working tree so you can inspect them. `--isolated` runs the same logic in a temporary worktree and does not touch your checkout.

**Jump to:** [Install](#install) · [Quick start](#quick-start) · [Use cases](#use-cases) · [Commands](#commands) · [Config](#config) · [Reference](#reference) · [CI](#ci)

## Why not just `git diff`?

The usual DIY flow is: run codegen, then `git diff`. That works, but you have to wire it yourself — remember every output path, diff against `HEAD` (not the index, or a staged file looks fine), and wipe old outputs so a file your generator stopped producing does not linger. Add a second generator and you are maintaining a script.

genguard keeps the rules beside the generated tree and only watches paths you declare:

- Drift is measured against **`HEAD`**. Staged-but-uncommitted generated files still fail until you commit them.
- Optional **`clean`** deletes declared outputs before the command runs.
- Several **groups** run in order from one config file.
- **`--since`** skips groups whose inputs did not change on this branch (see [Since flag](#since-flag)).
- **`--isolated`** checks committed files in a throwaway worktree.

```yaml
clean: true # wipe declared outputs first — catches files the generator no longer writes
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
    clean: false # overrides the top-level clean
```

Place `genguard.yaml` (or `genguard.yml`) next to your generated files — not inside an `outputs` directory. Stack-specific templates live in [examples/](examples/README.md). They are config only; your generator setup stays yours.

## Install

Pick one path below. After `go install`, put `$(go env GOPATH)/bin` on your `PATH`.

**Pinned release (Go 1.22+):**

```bash
go install github.com/wuddleko/genguard/cmd/genguard@v0.7.0
```

**This repository:**

```bash
make install
# or
go install ./cmd/genguard
```

**No Go** — needs `curl`, `awk`, and `sha256sum` or `shasum`. Download [install.sh](install.sh), read it, then run:

```bash
sh install.sh
sh install.sh v0.6.0
# or
GENGUARD_TAG=v0.6.0 sh install.sh
```

The script verifies the release binary against `checksums.txt` from the same tag. Piping `curl … | sh` is convenient but trusts both the script URL and that release; prefer a clone or a saved copy you have inspected. See `sh install.sh --help` for `BINDIR` and platform overrides.

### Windows

Releases are `genguard_*_windows_amd64.zip` only (no Windows arm64). Download [from Releases](https://github.com/wuddleko/genguard/releases) or run `install.sh` from **Git Bash** or **MSYS** — it uses `unzip` when available, otherwise `tar` on the zip. In **WSL** you get the Linux binary, not `genguard.exe`.

## Quick start

[Install](#install) genguard, then from a Git work tree that contains your config:

```bash
genguard check
```

**GitHub Actions** (use `fetch-depth: 0` on checkout if you use `--since`):

```yaml
- uses: wuddleko/genguard@v0.7.0
  with:
    all: true
```

Generator install steps and more CI recipes: [docs/ci.md](docs/ci.md).

A match prints `Generated files match the generators.` and exits `0`. On drift you get a summary, paths, and a diff, then exit `1` — regenerate and commit. Exit codes and stderr details: [Commands](#commands).

### Which command when?

| Goal | Command |
|------|---------|
| CI or “are we in sync?” | `genguard check` |
| Regenerate locally after editing sources | `genguard run` |
| Large monorepo, skip untouched generators | `genguard check --since origin/main` |
| Do not dirty the working tree | `genguard check --isolated` |

## Use cases

### Generated Go code

Set `command` to `go generate ./...` and list the directories it writes. Generated files must be committed; genguard fails if a fresh run would change them. [Template](examples/go-generate.yaml).

### Protobuf

Run `buf generate` and put the generated tree under `outputs`. Add `proto/` under `inputs` if you want `--since` to skip the group when protos are unchanged. [Template](examples/buf.yaml).

### SQL with sqlc

`sqlc generate` with the generated package paths in `outputs`. [Template](examples/sqlc.yaml).

### OpenAPI

Point `command` at your generator (the example uses `openapi-generator-cli`) and list the client or types directory. [Template](examples/openapi.yaml).

### Makefile-driven codegen

If codegen is `make generate` (or similar), list the tree it produces under `outputs`. [Template](examples/make.yaml).

### More than one config

In a monorepo, put a `genguard.yaml` beside each service. `genguard check --all` discovers and runs every config.

## Commands

Subcommands:

| Command | What it does |
|---------|----------------|
| **`check`** | Run generators, then fail if declared outputs differ from `HEAD`. |
| **`run`** | Run generators only; exit `0` on success. |
| **`version`** | Print the version. |

Shared flags:

| Flag | What it does |
|------|----------------|
| **`-c` / `--config`** | Config file. Otherwise search upward for `genguard.yaml` / `genguard.yml` in the repo. |
| **`--all`** | Run every config in the repository. [Discovery rules](#all-discovery). |
| **`--since <ref>`** | Skip groups with `inputs` when paths are unchanged vs `HEAD` and vs the merge-base. [Full rules](#since-flag). |
| **`--isolated`** | Check committed files in a temporary worktree (`check` only). |
| **`--json`** | Print results as JSON on stdout. [Schema](#json-output). |
| **`--verbose`** | Stream generator stdout/stderr to stderr while each command runs. |

`check` and `run` take **flags only**. A positional argument exits `2` with `unexpected argument` — pass the config with `--config`. Older releases ignored a positional path and trailing flags; do not rely on that.

Staged generated files still count as drift until committed. On `run` success you get `Generated files written.`

### Exit codes

| Exit | Meaning |
|------|---------|
| `0` | `check`: outputs match `HEAD` (skipped groups are fine). `run`: commands succeeded. |
| `1` | Drift (`check` only). |
| `2` | Config error, Git error, bad `--since` ref, or generator failure. Command failure wins over drift. |

### Output on stderr

Successful generator commands stay quiet even when another group drifted. Failed commands print the last 50 lines, then a **Summary**. Group labels are `name:`; with `--all`, `path/to/genguard.yaml: name:`.

With `--json`, the document is on stdout; human-readable Summary and `error:` lines stay on stderr. Details: [Command output](#command-output).

## Config

Without `--config`, genguard searches upward for `genguard.yaml` or `genguard.yml` inside a Git work tree. One filename per directory — not both. `--all` together with `--config` exits `2`.

Unknown top-level keys are errors.

### Top-level

| Key | Required | Description |
|-----|----------|-------------|
| **`groups`** | yes | Non-empty list of generator groups. |
| **`clean`** | no | Default `false`. Wipe each group’s `outputs` before its command unless the group overrides. |
| **`tools`** | no | Tool definitions groups can reference for version checks. |
| **`timeout`** | no | Default command/probe timeout for all groups (Go duration, e.g. `1m`). Probes use the same limit. |

### Per group (`groups[]`)

| Key | Required | Description |
|-----|----------|-------------|
| **`name`** | no | Label in logs; defaults to `groups[0]`, `groups[1]`, … Empty or whitespace uses that default. |
| **`command`** | yes | Shell command from the config directory. See [Shell](#shell). |
| **`outputs`** | yes | Non-empty paths or globs (relative to the config file). |
| **`inputs`** | no | Paths for `--since`. Omit to always run this group. |
| **`clean`** | no | Overrides top-level `clean` for this group. |
| **`tools`** | no | Subset of top-level `tools` names required before this group runs. |
| **`timeout`** | no | Overrides top-level `timeout` for this group (probes included). |

Validation edge cases (`clean: ~`, empty `inputs`, overlaps, and timeouts): [Config validation](#config-validation).

### Shell

Each group **`command`** runs from the config file’s directory via `sh -c`. On Windows, genguard uses `sh` or `bash` when either is on `PATH`; otherwise `%COMSPEC% /C`.

### Tool pins

Each top-level `tools` entry has **`name`** (required), optional **`version`** (exact pin; leading `v` ignored when comparing), and optional **`command`** (probe; default `name --version`).

Groups list tool names under their own **`tools`** key. Names not declared at the top level are config errors. Probes run before `clean` and the generator, using the group’s **timeout** (or the top-level default). Missing binaries or version mismatches fail the group without running its command. Results are cached per config.

On success or drift, the Summary may append detected versions, e.g. `protobuf: drift (1 modified); buf 1.32.0`. In `--json`, `want` is the pin and `have` is what the probe saw (no leading `v`); when there is no pin, `want` is omitted.

### Drift

After each command, paths under `outputs` are compared to `HEAD`:

| Kind | Meaning |
|------|---------|
| `modified` | Tracked file changed |
| `untracked` | New file, not gitignored |
| `missing` | Tracked file removed, or a listed **file** missing on disk — not a directory or glob path by itself |

Generated files should be tracked. Gitignored paths under `outputs` are not reported. Failed commands still produce a diff; see [Command output](#command-output).

Groups run in order and share one tree. `clean` behavior and overlap rules: [Config validation](#config-validation).

## Reference

### Since flag

For a group with **`inputs`**, genguard skips the group when its inputs, outputs, and config file match the working tree at both:

1. the merge-base of `HEAD` and the `--since` ref, and  
2. `HEAD`.

The group **still runs** if:

- a listed **file** output is missing on disk,
- `git diff` names any of those paths against either revision above, or
- any of those paths has an untracked file.

Groups without `inputs` always run. Skipped groups do not run their command and do not run `clean`. A bad ref exits `2` before any group runs.

`check --all --since <ref>` uses one merge-base for the whole run, then the same rules for each config and group.

CI notes (fetch depth, monorepos): [docs/ci.md](docs/ci.md).

### All discovery

| Mode | How configs are found |
|------|------------------------|
| `check --all` / `run --all` | Index plus non-ignored untracked files (`git ls-files`). |
| `check --all --isolated` | Committed tree at `HEAD` (`git ls-tree`). |

Skipped paths: `.git`, `vendor`, `node_modules`. Gitignored untracked configs are never listed. A config only in the index (not committed) appears in the first mode but not isolated mode; a committed config removed from the index may still appear in isolated mode. Discovery fails if Git cannot read a directory that might contain a config.

`--all --isolated` uses one temporary worktree per config.

### JSON output

Stdout is one JSON object:

- **`exit`** — process exit code  
- **`configs`** — array of `{ "path", "exit", "groups" }`  
- each **group**: `name`, `status`, `error`, `drifts` (`kind`, `path`), `tools` (`want`, `have`)

If the run stops before any config loads, stdout is still `{"exit": 2, "error": "...", "configs": []}`. Human `error:` lines remain on stderr. Unknown CLI flags print usage only.

### Command output

Summary lines use `command failed (exit N)`; empty command output adds `: no output`. When that is the only failure, stderr ends with `error: command failed (exit N)`. The group’s `error` field in `--json` matches that sentence.

`--verbose` prefixes the first streamed line with the same label as the Summary (`name:` or `config: name:`).

**Interrupt:** before any group runs, `error: interrupted` and exit `2`. After that, Summary covers finished groups, then a final `error: interrupted`.

### Config validation

- **YAML empties:** missing keys use defaults; `null` / blank strings where a value is required are errors.  
- **`inputs`:** `[]` is an error; omit the key to always run the group.  
- **`clean` / `timeout`:** `clean: ~` and `timeout: ~` are invalid. Timeout must be a positive Go duration (`200ms`, `1m`); `0` or invalid strings are errors. On timeout the group fails with exit `2`, drift is still computed, and `clean` is not rolled back.  
- **`outputs`:** entries must not start with `:`; globs follow Git rules (`*`, `?`, `[...]` match `/`).  
- **Overlap:** two specs (within one file or across `--all`) that can name the same path are a load error; commands do not run. Isolated mode still reads committed specs for overlap. Parse errors are per file; valid outputs from other files still count toward overlap.  
- **`clean: true`:** removes declared outputs before the command (directories recreated empty, files deleted). Refuses `.`, `..`, absolute paths, paths outside the config directory, symlinks, `.git`, and the config file. A glob matching a directory is an error. Failed commands do not restore a wipe.

## Security

Scope of what a run can do, and how to report issues: [SECURITY.md](SECURITY.md).

## CI

The composite action installs the release named by `uses:` and runs `genguard check`. It does not install `buf`, `sqlc`, or other generators — add those steps yourself. Snippets and per-tool setup: [docs/ci.md](docs/ci.md).

## Development

```bash
make test              # go test ./...
make lint              # go vet ./...
make build             # bin/genguard
make validate-examples # example YAML templates load cleanly
```

Tests need `git` and `python3`. Internals: [docs/design.md](docs/design.md).

## License

MIT — see [LICENSE](LICENSE).
