# CI cookbook

`genguard check` is meant to run in CI after checkout, inside a git work tree. It re-runs your declared generator commands and fails if `git diff HEAD` would show changes under the declared `outputs` (working tree vs committed files, including staged but uncommitted generated output). Gitignored files under `outputs` are not reported as untracked — commit the generated files.

Groups run in order. Every group runs even when an earlier one fails or drifts; on failure genguard prints a **Summary** (one line per group, with drift kinds), then **Drift** details and diffs. Later groups see the working tree as earlier groups left it. When an earlier group fails after `clean`, paths it wiped and a later group leaves untouched stay out of that later group's drift. A later command that writes those paths is checked as usual. Prefer disjoint `outputs` so a group that drifts cannot change the tree the next group checks.

Exit codes:

| Code | Meaning |
|---|---|
| `0` | Generated files match |
| `1` | Drift detected |
| `2` | Config, git, or generator command error |

## GitHub Actions (install from release)

Pin a tag and download the matching archive for your runner OS/arch. Archive names use the version **without** the `v` prefix (`genguard_0.2.0_linux_amd64.tar.gz`); the GitHub download path still uses the tag (`v0.2.0`). Releases also include `checksums.txt`.

```yaml
name: Generated files

on:
  pull_request:
  push:
    branches: [main]

jobs:
  genguard:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install genguard
        env:
          GENGUARD_TAG: v0.2.0
        run: |
          GENGUARD_VERSION=${GENGUARD_TAG#v}
          curl -fsSL \
            "https://github.com/wuddleko/genguard/releases/download/${GENGUARD_TAG}/genguard_${GENGUARD_VERSION}_linux_amd64.tar.gz" \
            -o genguard.tar.gz
          tar xzf genguard.tar.gz genguard
          sudo install genguard /usr/local/bin/genguard

      - name: Verify generated files
        run: genguard check
```

Replace `GENGUARD_TAG` with a [release tag](https://github.com/wuddleko/genguard/releases). Archive names follow `genguard_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows; binary `genguard.exe`).

### macOS runners

Use `darwin_arm64` on `macos-latest`, or `darwin_amd64` for Intel:

```yaml
      - name: Install genguard
        env:
          GENGUARD_TAG: v0.2.0
        run: |
          GENGUARD_VERSION=${GENGUARD_TAG#v}
          curl -fsSL \
            "https://github.com/wuddleko/genguard/releases/download/${GENGUARD_TAG}/genguard_${GENGUARD_VERSION}_darwin_arm64.tar.gz" \
            -o genguard.tar.gz
          tar xzf genguard.tar.gz genguard
          sudo install genguard /usr/local/bin/genguard
```

## GitHub Actions (Go install)

Fine when the job already uses Go and you want a tagged module version without downloading archives:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Install genguard
        run: go install github.com/wuddleko/genguard/cmd/genguard@v0.2.0

      - name: Verify generated files
        run: genguard check
```

Ensure `$(go env GOPATH)/bin` is on `PATH` (true by default on GitHub-hosted runners after `setup-go`).

## Monorepo with one config at repo root

```yaml
      - uses: actions/checkout@v4
      - run: go install github.com/wuddleko/genguard/cmd/genguard@v0.2.0
      - run: genguard check
```

Place `genguard.yaml` or `genguard.yml` at the repository root. Paths in `outputs` are relative to that file. A directory holds one of those names; both files make `genguard check` and `genguard check --all` exit `2`.

## Monorepo with config per service

`genguard check --all` discovers every config under the repository root, one `genguard.yaml` or `genguard.yml` per directory. It skips directories named `.git`, `vendor`, and `node_modules`, and it does not read `.gitignore`, so a config inside an ignored directory still runs. A passing run prints each config path and a totals line. Both names in one directory exit `2`.

```yaml
      - uses: actions/checkout@v4
      - run: go install github.com/wuddleko/genguard/cmd/genguard@v0.2.0
      - run: genguard check --all
```

To check specific services, pass each config (`-c` is the same flag):

```yaml
      - run: genguard check --config services/api/genguard.yaml
      - run: genguard check -c services/worker/genguard.yaml
```

Each config’s commands run in that config’s directory, not the workflow’s working directory. `--all` runs configs in path order on the shared working tree. A path left untouched after an earlier config fails following `clean` stays out of later configs' drift. Keep `outputs` disjoint across configs.

## Matrix over example templates

This repository validates that every file under `examples/*.yaml` parses:

```yaml
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: make validate-examples
```

## What to commit

- Source files your generator reads (`.proto`, OpenAPI spec, SQL queries, etc.)
- The generator command in `genguard.yaml` or `genguard.yml`
- The generated output paths listed under `outputs` (tracked, not gitignored)

Do **not** rely on CI to mutate the repo. `genguard check` leaves the working tree as the generator wrote it and does not `git add`. With `clean: true`, it also deletes declared outputs before regenerating. Developers regenerate locally, commit, and push.

## When check fails in CI

The log shows a **Summary** (per-group status), **Drift** lines, and a git diff, for example:

```
Summary
  openapi: drift (1 modified)
1 group: 0 ok, 1 drift, 0 error

Drift
[modified] openapi: generated/models.py

diff --git a/generated/models.py ...

error: 1 generated path drifted; commit the generator output or fix the command
```

Fix locally:

```bash
# run the same command from genguard.yaml, or your usual make target
make generate
git add generated/
git commit -m "regenerate"
```

## Tool-specific configs

Swap the `command` for your stack; keep `outputs` aligned with what the tool writes. `name` is optional (defaults to `groups[N]`).

```yaml
groups:
  - name: protobuf
    command: buf generate
    outputs:
      - gen/

  - name: sqlc
    command: sqlc generate
    outputs:
      - internal/db/

  - name: openapi
    command: openapi-generator-cli generate -i api.yaml -g go -o gen/
    outputs:
      - gen/
```

See [examples/README.md](../examples/README.md) for copy-paste `genguard.yaml` templates (`buf`, `sqlc`, `go generate`, etc.).

## Releasing genguard itself

Tag a version to trigger GoReleaser:

```bash
git tag v0.2.0
git push origin v0.2.0
```

The [release workflow](../.github/workflows/release.yml) publishes archives for Linux and macOS (`amd64`, `arm64`) and Windows (`amd64`), plus `checksums.txt`. Each archive includes the binary, `LICENSE`, `README.md`, `SECURITY.md`, `docs/ci.md`, `genguard.example.yaml`, and `examples/`. Archive filenames use the tag without the `v` (`genguard_0.2.0_linux_amd64.tar.gz`). On Windows the binary is `genguard.exe`; group commands use `sh`/`bash` when present, otherwise `%COMSPEC% /C` (typically `cmd.exe`).
