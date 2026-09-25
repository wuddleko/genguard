# genguard

genguard re-runs the commands in `genguard.yaml`.

- `check` — run the generators, leave their files in place, and fail if the declared outputs differ from `HEAD`. A file that is only staged still fails until it is committed.
- `run` — run the generators and leave their files in place. A success exits 0.
- `version` — print the version.
- `-c`, `--config` — config file to use. Otherwise genguard walks up from the current directory for `genguard.yaml` or `genguard.yml`.
- `--all` — every config under the repository.
- `--since` — skip a group that declares `inputs` when those inputs, its outputs, and the config are unchanged since the merge-base of that ref and `HEAD`.
- `--isolated` — check the committed files in a temporary worktree and leave your checkout alone. `check` only.
- `--json` — print the result as JSON.

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

## Install

With Go 1.22+:

```bash
go install github.com/wuddleko/genguard/cmd/genguard@v0.5.0
```

`$(go env GOPATH)/bin` has to be on your `PATH`. From a checkout of this repository: `make install` or `go install ./cmd/genguard`.

Without Go, [install.sh](install.sh) downloads the release for this machine, checks the archive against `checksums.txt`, and installs into a directory already on `PATH`. From a checkout: `sh install.sh`. Pass a tag, or set `GENGUARD_TAG`, for another release. `sh install.sh --help` lists location and platform options.

## Config

With no `--config`, genguard walks up from the current directory for `genguard.yaml` or `genguard.yml`, inside a git work tree. A directory holds one of those names. Both files, or `--all` together with `--config`, exit 2.

Top-level keys are `groups` (required, non-empty) and `clean`. An unknown key is an error.

- **`name`** — optional label. The fallback is `groups[0]`, `groups[1]`, and so on.
- **`command`** — required. Run with `sh -c` from the config directory. On Windows, `sh` or `bash` on `PATH`; otherwise `%COMSPEC% /C`.
- **`outputs`** — required, non-empty git pathspecs, relative to the config file.
- **`inputs`** — optional pathspecs for `--since`. Omit the key and the group runs every time. An empty list is an error.
- **`clean`** — optional, default `false`. Delete the outputs before the command. A top-level `clean` applies to every group; a group can override it.

After the command, genguard compares `HEAD` to the working tree under `outputs`:

| What you see | What happened |
|---|---|
| `modified` | A tracked file changed |
| `untracked` | A new file that is not gitignored |
| `missing` | A tracked file under `outputs` is gone, or a listed **file** is absent. A directory or a glob is not, by itself, a missing path |

Generated paths have to be tracked. Gitignored files under `outputs` stay out of the report. A failed command is still diffed, and that group counts as an error (exit 2).

`clean: true` removes the declared outputs before the command: a directory is recreated empty, a file is removed, a glob removes the tracked and untracked matches. It refuses `.`, `..`, absolute paths, anything outside the config directory, symlinks, and a delete that would take `.git` or the config file. A failed command does not restore the wipe.

Groups run in order and share the tree, so keep `outputs` from overlapping. `--all --isolated` gives each config its own worktree. `--isolated` is only valid with `check`. A bad `--since` ref exits 2 before any group runs. An unchanged group is `skipped`: its command does not run, and `clean` does not delete its outputs.

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
