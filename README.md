# regen

**Keep generated files honest in CI.**

`regen check` re-runs your codegen commands and fails if the result does not match what is already committed. Hand-edited generated files, stale commits after a schema change, or a forgotten `make generate` — all caught before merge.

It works with any generator: protobuf, OpenAPI, sqlc, `go generate`, Make targets, or anything else you can run in a shell.

## Quick start

1. Add a `regen.yaml` (or `regen.yml`) beside the directory that holds generated files (or copy a template from [examples/](examples/README.md)):

```yaml
groups:
  - name: protobuf
    command: buf generate
    outputs:
      - gen/
```

2. From a git work tree, run the check locally or in CI:

```bash
regen check
```

If outputs drift, `regen check` prints what changed and exits non-zero.

```
regen check [-c|--config path/to/regen.yaml]
regen version
```



## How it works

Groups run in order. If a group's `command` fails, later groups are not run.

For each group, `regen check`:

1. If `clean` is true, deletes the declared `outputs` (then recreates empty directories)
2. Runs `command` from the directory that contains the config file (`sh -c` on Unix)
3. Compares git HEAD to the working tree under the declared `outputs`
4. Reports drift and exits `1` if anything changed

The working tree is left as the generator left it — same idea as `make generate && git diff --exit-code HEAD`, but with explicit output paths and clearer errors. The check does not run `git add`. Staged generated files still fail until they are committed.

Generated paths must be tracked. Gitignored files under `outputs` are not reported as untracked.

On Windows, `command` runs with `sh -c` or `bash -c` when those shells are on `PATH` (Git Bash), otherwise `%COMSPEC% /C` (typically `cmd.exe`). POSIX recipes need a POSIX shell.

| Drift kind | What it means |
|---|---|
| `modified` | A tracked generated file exists and changed |
| `untracked` | The generator wrote a file that is not in git (and is not gitignored) |
| `missing` | A tracked file under `outputs` was deleted, or a listed **file** path does not exist after the command. Directory and glob specs (`gen/`, `*.go`) are not themselves reported as missing. |

| Exit code | Meaning |
|---|---|
| `0` | Generated files match |
| `1` | Drift detected |
| `2` | Config, git, or generator command error |

## Install

### Prebuilt binary (recommended for CI)

Download an archive from [GitHub Releases](https://github.com/regen-check/regen/releases) and put `regen` on your `PATH`. Release archives are named with the version **without** the `v` prefix; the download URL still uses the git tag. Each release also publishes `checksums.txt`.

```bash
REGEN_TAG=v0.1.0
REGEN_VERSION=${REGEN_TAG#v}
curl -fsSL \
  "https://github.com/regen-check/regen/releases/download/${REGEN_TAG}/regen_${REGEN_VERSION}_linux_amd64.tar.gz" \
  -o regen.tar.gz
tar xzf regen.tar.gz regen
sudo install regen /usr/local/bin/regen
```

Adjust OS and arch in the filename (`darwin_arm64`, `linux_amd64`, `windows_amd64.zip`, etc.). On Windows the binary is `regen.exe`.

### From source

Requires Go 1.22+.

```bash
make install
# or, from this repository
go install ./cmd/regen
# or, from a published module version
go install github.com/regen-check/regen/cmd/regen@latest
```

Ensure `$(go env GOPATH)/bin` is on your `PATH`.

## Configuration

Place `regen.yaml` or `regen.yml` beside the directory that holds generated files, or pass an explicit path:

```bash
regen check --config path/to/regen.yaml
regen check -c path/to/regen.yaml
```

If you omit `--config` / `-c`, regen walks up from the current directory until it finds `regen.yaml` or `regen.yml`. The directory that contains the config must be a git work tree (or inside one).

Each group has:

- **`name`** — optional; label used in error output. Defaults to `groups[N]`.
- **`command`** — shell command that regenerates files (run from the config directory)
- **`outputs`** — git pathspecs, relative to the config file's directory, for generated files to check
- **`clean`** — optional; default `false`. If `true`, delete those outputs before running `command`, so files the generator no longer writes fail the check. Also valid at the top level of the config as the default for every group.

`clean` is destructive. Use it only on generated-only directories. It refuses `.`, `..`, globs, absolute paths, paths that escape the config directory, symlinks, and any tree that would delete `.git` or the config file (`regen.yaml` / `regen.yml`). If `command` fails after a wipe, the error says so; outputs are not restored.

See [regen.example.yaml](regen.example.yaml) and the stack-specific templates in [examples/](examples/README.md):

- [go-generate.yaml](examples/go-generate.yaml) — `go generate ./...`
- [buf.yaml](examples/buf.yaml) — protobuf via `buf generate`
- [openapi.yaml](examples/openapi.yaml) — OpenAPI clients and types
- [sqlc.yaml](examples/sqlc.yaml) — SQL → Go via `sqlc generate`
- [make.yaml](examples/make.yaml) — Makefile-driven codegen

These templates are config only. regen does not ship generators; it runs whatever your `command` declares.

## CI

GitHub Actions snippets, monorepo patterns, and tool-specific setup: [docs/ci.md](docs/ci.md).

## Development

```bash
make test
make lint   # go vet ./...
```

Tests require `git` and `python3` (used by the test fixtures' sample generator).

## What regen is for

regen is a small, language-agnostic guardrail: declare how files are generated, declare where they land, and let CI prove they stay in sync.

It is not a replacement for secret scanning or tool-specific commands like `sqlc diff` or `buf breaking`. Those solve different problems. regen is the generic "re-run the generator and compare git" step that fits any stack.

## License

MIT — see [LICENSE](LICENSE).
