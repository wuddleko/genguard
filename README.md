# genguard

`genguard check` runs your generators in the working tree and fails if the declared outputs differ from `HEAD`. It leaves that tree as the generators left it. Uncommitted input edits are part of the run. A generated file that is only staged still fails until that commit exists. In CI the checkout is usually clean, so `HEAD` is the commit under test: the pull request branch tip, or the push that started the job.

`genguard check --isolated` checks the committed files in a throwaway worktree and leaves your checkout alone.

A hand-edited `.pb.go`, or a schema change nobody regenerated, shows up as a diff. With `clean: true`, genguard deletes the declared outputs first, so a file the generator stopped writing fails the check too. The command can be anything you can run in a shell: `buf generate`, `sqlc generate`, `go generate`, a Make target, your own script.

## Run it

Put a `genguard.yaml` (or `genguard.yml`) next to the generated files and outside any directory listed in `outputs`. `clean: true` deletes that directory, and a delete that would take the config file with it is refused. Templates for the usual tools (`go generate`, buf, OpenAPI, sqlc, Make) are in [examples/](examples/README.md). Those files are config only. The generator stays yours.

```yaml
groups:
  - name: protobuf
    command: buf generate
    outputs:
      - gen/
```

From a git checkout:

```bash
genguard check
```

A match prints `Generated files match the generators.` and exits 0. Drift prints a summary, the paths, and a diff, then exits 1:

```
Summary
  protobuf: drift (1 modified)
1 group: 0 ok, 1 drift, 0 error

Drift
[modified] protobuf: gen/foo.pb.go

diff --git a/gen/foo.pb.go ...

error: 1 generated path drifted; commit the generator output or fix the command
```

Regenerate and commit. `git add` alone still fails, because the diff is the working tree against `HEAD`.

```
genguard check [-c|--config path] [--since ref] [--isolated] [--json]
genguard check --all [--since ref] [--isolated] [--json]
genguard run [-c|--config path] [--since ref] [--json]
genguard run --all [--since ref] [--json]
genguard version
```

`-c` is `--config`.

## Install

With Go 1.22+:

```bash
go install github.com/wuddleko/genguard/cmd/genguard@v0.5.0
```

`$(go env GOPATH)/bin` has to be on your `PATH`.

From a checkout of this repository:

```bash
make install
# or
go install ./cmd/genguard
```

Without Go, [install.sh](install.sh) downloads the release for this machine, checks the archive against `checksums.txt`, and installs into a directory already on `PATH`. From a checkout of this repository:

```bash
sh install.sh
```

Pass a tag, or set `GENGUARD_TAG`, to choose another release. Run `sh install.sh --help` for install location and platform options.

## Config

With no `--config`, genguard walks up from the current directory until it finds `genguard.yaml` or `genguard.yml`. That file has to sit inside a git work tree. A directory holds one of those names. Both files in the same directory make `genguard check` and `genguard check --all` exit 2. `--config` still loads the path you gave it. `--all` and `--config` together exit 2.

`genguard check --all` runs every config under the repository root, in path order, on the same working tree. It skips directories named `.git`, `vendor`, and `node_modules`. It does not read `.gitignore`, so a config inside an ignored directory still runs. A clean run prints each config path and a totals line.

`--isolated` uses the committed copy of the config. Groups in that file still share the throwaway worktree. `--all --isolated` finds configs tracked at `HEAD`, including one deleted in the checkout, and skips a config that is not committed. Each config gets its own worktree, so a failed `clean` in one file does not wipe files another config is judging.

`genguard run` runs the same commands and leaves the tree as the generator wrote it. A success prints `Generated files written.` It does not compare with `HEAD`, so it never exits 1. A failed command still exits 2. A skip under `--since` does not run `clean`. `--isolated` is not valid with `run`. `genguard run --all` uses one checkout for every config, so a `clean` in one file still deletes paths a later config is about to use.

`--json` prints one JSON object on stdout and skips the human report. The object has `exit` and `configs`. Each config has `path` (relative to the repository root), `exit`, and `groups`, plus `error` when loading that config failed or an isolated worktree could not be removed. Read `exit`. `error` is extra detail, and a cleanup failure sets both (`exit` is 2). A group has `name`, `status` (`ok`, `drift`, `error`, `skipped`), and, when present, `error` and `drifts` (`kind`, `path`). A drift `path` is relative to the repository root. Diffs stay in the human report. A failure before any config runs prints `error:` on stderr and leaves stdout empty. `genguard run --json` has no `drift` status: a successful command is `ok`.

Top-level keys are `groups` and `clean`. `groups` is required and non-empty. An unknown key is a config error.

A group has:

- **`name`** — label in the output. Optional. The fallback is `groups[0]`, `groups[1]`, and so on.
- **`command`** — required. The shell command. genguard runs it with `sh -c` from the directory that contains the config, and it will run whatever that command is. On Windows it uses `sh` or `bash` when one of those is on `PATH` (Git Bash); otherwise it uses `%COMSPEC% /C` (`cmd.exe` when that variable is unset). A POSIX recipe needs a POSIX shell.
- **`outputs`** — required, non-empty. Git pathspecs, relative to the config file, for the files to compare.
- **`inputs`** — optional git pathspecs, relative to the config file, for the sources that feed the command. Used by `--since`. Leave the key off and the group runs on every check. An empty list is a config error.
- **`clean`** — optional, default `false`. Delete those outputs before the command runs. A `clean` at the top of the file applies to every group; a group can override it.

After the command, genguard compares `HEAD` to the working tree under `outputs`:

| What you see | What happened |
|---|---|
| `modified` | A tracked file is still there, and it changed |
| `untracked` | The generator wrote a file git does not know about, and the file is not gitignored |
| `missing` | A tracked file under `outputs` is gone, or a **file** you listed is not there after the command. A directory or a glob (`gen/`, `*.go`) is not, by itself, a missing path |

Generated paths have to be tracked. Gitignored files under `outputs` are left out of the report.

A command that fails is still diffed. The group counts as an error, so the run exits 2. Drift lists files the command rewrote. A `clean: true` wipe the command never rewrote is left out; the error already says those outputs are gone. The summary line names both facts (`protobuf: error (...); drift (1 modified)`), and the totals line counts the group once, as an error. When this is the only failure, the final line stays the command error. When another group drifted on its own, the final line is `error: 1 group failed; 1 group drifted`.

### `clean`

Turn this on when a generator might stop emitting a file. Without it, `old.go` just sits there and a diff never mentions it. With `clean: true`, genguard deletes the declared outputs, runs the command, and reports `old.go` as missing if it did not come back.

The delete is real.

- A directory output is removed and recreated empty.
- A file output is removed.
- A glob removes the tracked and untracked files git matches, and leaves ignored files alone. `*_queries.sql.go` can live next to Go you wrote by hand.

It refuses `.`, `..`, absolute paths, anything that escapes the config directory, symlinks, and any delete that would take `.git` or the config file with it. Every output is checked before anything is removed, so one refused path leaves the tree as it was. If the command fails after a wipe, the error says the outputs are already gone. They are not put back.

## `--since`

`genguard check --since origin/main` resolves the merge-base of that ref and `HEAD`. A group with `inputs` runs when the working tree differs from that merge-base, or from `HEAD`, under its inputs, its outputs, or the config file. A committed change on the branch counts, and so does a staged edit, an unstaged edit, or an untracked file that is not gitignored. A hand-edit of a generated file still runs that group. So does a change to the group's command or `clean`, a new untracked config, and a declared output file that is not on disk. Directory outputs and globs are not that last check. A group with no `inputs` still runs.

```yaml
groups:
  - name: sqlc
    command: sqlc generate
    inputs:
      - queries/
      - sqlc.yaml
    outputs:
      - internal/db/
  - name: protobuf
    command: buf generate
    inputs:
      - proto/
    outputs:
      - gen/
```

An unchanged group is `skipped`. Its command does not run, and `clean: true` does not delete its outputs. It is not counted as ok. A passing check prints the skips after the success sentence:

```
Generated files match the generators.
  sqlc: OK
  protobuf: skipped
2 groups: 1 ok, 0 drift, 0 error, 1 skipped
```

Without `--since`, every group runs and `inputs` is ignored.

`genguard check --all --since origin/main` uses one merge-base for the repository, then the same rule per group. A passing run prints each config path and the config totals, then prints again each config that skipped a group:

```
Generated files match the generators.
api/genguard.yaml
genguard.yaml
2 configs: 2 ok, 0 drift, 0 error

api/genguard.yaml
  api: skipped
1 group: 0 ok, 0 drift, 0 error, 1 skipped

genguard.yaml
  sqlc: OK
  protobuf: skipped
  plain: OK
3 groups: 2 ok, 0 drift, 0 error, 1 skipped
```

A missing or unrelated ref exits 2 before any group runs:

```
error: bad --since ref: fatal: Not a valid object name not-a-ref
```

## More than one group

Groups run in the order you listed them. Drift or a command error in an earlier group still lets the rest go. Each group sees the tree the previous group left. Give them outputs that do not overlap, or a drift in one group changes what the next group is judging. `genguard check --all` follows the same rule across config files.

If an earlier group has `clean: true` and the command fails, paths it wiped and a later group never writes stay out of that later group's drift. A later command that does write them is checked as usual.

These are the exit codes for `genguard check`. A skip does not add a code. `genguard run` uses `0` and `2` only.

| Exit code | Meaning |
|---|---|
| `0` | Generated files match, including when some groups were skipped |
| `1` | Drift |
| `2` | Config, git, or the generator command failed. A command error wins over drift |

## Security

What a run is allowed to do, and how to report a vulnerability: [SECURITY.md](SECURITY.md).

## CI

GitHub Actions snippets, monorepo layouts, and per-tool setup are in [docs/ci.md](docs/ci.md).

## Development

```bash
make test
make lint   # go vet ./...
```

Tests need `git` and `python3`. The fixtures use `python3` as a stand-in generator.

## License

MIT. See [LICENSE](LICENSE).
