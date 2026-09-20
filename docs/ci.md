# CI cookbook

`regen check` is meant to run in CI after checkout, inside a git work tree. It re-runs your declared generator commands and fails if `git diff HEAD` would show changes under the declared `outputs` (working tree vs committed files, including staged but uncommitted generated output). Gitignored files under `outputs` are not reported as untracked — commit the generated files.

Groups run in order. Every group runs even when an earlier one fails or drifts; regen prints a per-group summary before drift details. Later groups see the working tree as earlier groups left it, including files wiped by `clean`. Prefer disjoint `outputs` so a failed or drifting group cannot look like drift in the next one.

Exit codes:

| Code | Meaning |
|---|---|
| `0` | Generated files match |
| `1` | Drift detected |
| `2` | Config, git, or generator command error |

## GitHub Actions (install from release)

Pin a tag and download the matching archive for your runner OS/arch. Archive names use the version **without** the `v` prefix (`regen_0.1.0_linux_amd64.tar.gz`); the GitHub download path still uses the tag (`v0.1.0`). Releases also include `checksums.txt`.

```yaml
name: Generated files

on:
  pull_request:
  push:
    branches: [main]

jobs:
  regen:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Install regen
        env:
          REGEN_TAG: v0.1.0
        run: |
          REGEN_VERSION=${REGEN_TAG#v}
          curl -fsSL \
            "https://github.com/wuddleko/regen/releases/download/${REGEN_TAG}/regen_${REGEN_VERSION}_linux_amd64.tar.gz" \
            -o regen.tar.gz
          tar xzf regen.tar.gz regen
          sudo install regen /usr/local/bin/regen

      - name: Verify generated files
        run: regen check
```

Replace `REGEN_TAG` with a [release tag](https://github.com/wuddleko/regen/releases). Archive names follow `regen_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows; binary `regen.exe`).

### macOS runners

Use `darwin_arm64` on `macos-latest`, or `darwin_amd64` for Intel:

```yaml
      - name: Install regen
        env:
          REGEN_TAG: v0.1.0
        run: |
          REGEN_VERSION=${REGEN_TAG#v}
          curl -fsSL \
            "https://github.com/wuddleko/regen/releases/download/${REGEN_TAG}/regen_${REGEN_VERSION}_darwin_arm64.tar.gz" \
            -o regen.tar.gz
          tar xzf regen.tar.gz regen
          sudo install regen /usr/local/bin/regen
```

## GitHub Actions (Go install)

Fine when the job already uses Go and you want a tagged module version without downloading archives:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Install regen
        run: go install github.com/wuddleko/regen/cmd/regen@v0.1.0

      - name: Verify generated files
        run: regen check
```

Ensure `$(go env GOPATH)/bin` is on `PATH` (true by default on GitHub-hosted runners after `setup-go`).

## Monorepo with one config at repo root

```yaml
      - uses: actions/checkout@v4
      - run: go install github.com/wuddleko/regen/cmd/regen@v0.1.0
      - run: regen check
```

Place `regen.yaml` or `regen.yml` at the repository root. Paths in `outputs` are relative to that file.

## Monorepo with config per service

Run one check per config (`-c` is the same flag):

```yaml
      - uses: actions/checkout@v4
      - run: go install github.com/wuddleko/regen/cmd/regen@v0.1.0
      - run: regen check --config services/api/regen.yaml
      - run: regen check -c services/worker/regen.yaml
```

Each config’s commands run in that config’s directory, not the workflow’s working directory.

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
- The generator command in `regen.yaml` or `regen.yml`
- The generated output paths listed under `outputs` (tracked, not gitignored)

Do **not** rely on CI to mutate the repo. `regen check` leaves the working tree as the generator wrote it and does not `git add`. With `clean: true`, it also deletes declared outputs before regenerating. Developers regenerate locally, commit, and push.

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
# run the same command from regen.yaml, or your usual make target
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

See [examples/README.md](../examples/README.md) for copy-paste `regen.yaml` templates (`buf`, `sqlc`, `go generate`, etc.).

## Releasing regen itself

Tag a version to trigger GoReleaser:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The [release workflow](../.github/workflows/release.yml) publishes archives for Linux and macOS (`amd64`, `arm64`) and Windows (`amd64`), plus `checksums.txt`. Archive filenames use the tag without the `v` (`regen_0.1.0_linux_amd64.tar.gz`). On Windows the binary is `regen.exe`; group commands use `sh`/`bash` when present, otherwise `%COMSPEC% /C` (typically `cmd.exe`).
