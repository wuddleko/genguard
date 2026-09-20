# genguard

**Keep generated files honest in CI.**

`genguard check` re-runs your codegen commands and fails if the result does not match what is already committed. Hand-edited generated files, stale commits after a schema change, or a forgotten `make generate` — all caught before merge.

It works with any generator: protobuf, OpenAPI, sqlc, `go generate`, Make targets, or anything else you can run in a shell.

## Quick start

1. Add a `genguard.yaml` (or `genguard.yml`) beside the directory that holds generated files (or copy a template from [examples/](examples/README.md)):

```yaml
groups:
  - name: protobuf
    command: buf generate
    outputs:
      - gen/
```

2. From a git work tree, run the check locally or in CI:

```bash
genguard check
```

If outputs drift, `genguard check` prints what changed and exits non-zero:

```
[modified] protobuf: gen/foo.pb.go

diff --git a/gen/foo.pb.go ...
```

```
genguard check [-c|--config path/to/genguard.yaml]
genguard version
```



## How it works

Groups run in order. If a group's `command` fails, later groups are not run.

For each group, `genguard check`:

1. If `clean` is true, deletes the declared `outputs` (then recreates empty directories)
2. Runs `command` from the directory that contains the config file (`sh -c` on Unix)
3. Compares git HEAD to the working tree under the declared `outputs`
4. Reports drift and exits `1` if anything changed

This is a **reproducibility check**: it asks whether re-running the generator on the current branch reproduces what is already committed. It does not compare your branch to a PR target branch or merge base.

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

Download an archive from [GitHub Releases](https://github.com/wuddleko/genguard/releases) and put `genguard` on your `PATH`. Release archives are named with the version **without** the `v` prefix; the download URL still uses the git tag. Each release also publishes `checksums.txt`.

```bash
GENGUARD_TAG=v0.1.0
GENGUARD_VERSION=${GENGUARD_TAG#v}
curl -fsSL \
  "https://github.com/wuddleko/genguard/releases/download/${GENGUARD_TAG}/genguard_${GENGUARD_VERSION}_linux_amd64.tar.gz" \
  -o genguard.tar.gz
tar xzf genguard.tar.gz genguard
sudo install genguard /usr/local/bin/genguard
```

Adjust OS and arch in the filename (`darwin_arm64`, `linux_amd64`, `windows_amd64.zip`, etc.). On Windows the binary is `genguard.exe`.

### From source

Requires Go 1.22+.

```bash
make install
# or, from this repository
go install ./cmd/genguard
# or, from a published module version
go install github.com/wuddleko/genguard/cmd/genguard@latest
```

Ensure `$(go env GOPATH)/bin` is on your `PATH`.

## Configuration

Place `genguard.yaml` or `genguard.yml` beside the directory that holds generated files, or pass an explicit path:

```bash
genguard check --config path/to/genguard.yaml
genguard check -c path/to/genguard.yaml
```

If you omit `--config` / `-c`, genguard walks up from the current directory until it finds `genguard.yaml` or `genguard.yml`. The directory that contains the config must be a git work tree (or inside one).

Each group has:

- **`name`** — optional; label used in error output. Defaults to `groups[N]`.
- **`command`** — shell command that regenerates files (run from the config directory via `sh -c` on Unix). Treat it like CI workflow code: genguard executes whatever the config declares.
- **`outputs`** — git pathspecs, relative to the config file's directory, for generated files to check
- **`clean`** — optional; default `false`. If `true`, delete those outputs before running `command`, so files the generator no longer writes fail the check. Also valid at the top level of the config as the default for every group.

Without `clean`, a generator that stops producing `old.go` leaves the stale file in place and a plain `git diff` may not notice. With `clean: true`, genguard deletes `gen/` first, re-runs the generator, and reports the missing `old.go` as drift.

`clean` is destructive. Use it only on generated-only directories. It refuses `.`, `..`, globs, absolute paths, paths that escape the config directory, symlinks, and any tree that would delete `.git` or the config file (`genguard.yaml` / `genguard.yml`). If `command` fails after a wipe, the error says so; outputs are not restored.

See [genguard.example.yaml](genguard.example.yaml) and the stack-specific templates in [examples/](examples/README.md):

- [go-generate.yaml](examples/go-generate.yaml) — `go generate ./...`
- [buf.yaml](examples/buf.yaml) — protobuf via `buf generate`
- [openapi.yaml](examples/openapi.yaml) — OpenAPI clients and types
- [sqlc.yaml](examples/sqlc.yaml) — SQL → Go via `sqlc generate`
- [make.yaml](examples/make.yaml) — Makefile-driven codegen

These templates are config only. genguard does not ship generators; it runs whatever your `command` declares.

## CI

GitHub Actions snippets, monorepo patterns, and tool-specific setup: [docs/ci.md](docs/ci.md).

## Development

```bash
make test
make lint   # go vet ./...
```

Tests require `git` and `python3` (used by the test fixtures' sample generator).

## What genguard is for

genguard is a small, language-agnostic guardrail: declare how files are generated, declare where they land, and let CI prove they stay in sync.

You could run `make generate && git diff --exit-code HEAD`, but genguard adds:

- **Scoped outputs** — check only the paths you declare, not the whole repo
- **Multiple generators** — one config, one CI step
- **Missing and untracked detection** — not just modified files
- **Stale artifact detection** — with `clean: true`, files a generator stopped writing are caught
- **Clearer CI output** — drift kinds and diffs for the paths you declared

It is not a replacement for secret scanning or tool-specific commands like `sqlc diff` or `buf breaking`. Those solve different problems. genguard is the generic "re-run the generator and compare git" step that fits any stack.

## License

MIT — see [LICENSE](LICENSE).
