# CI cookbook

`genguard check` is meant to run in CI after checkout, inside a git work tree. It re-runs your declared generator commands and fails if `git diff HEAD` would show changes under the declared `outputs` (working tree vs committed files, including staged but uncommitted generated output). Gitignored files under `outputs` are not reported as untracked — commit the generated files.

Groups run in order. Without `--since`, every group runs even when an earlier one fails or drifts. On failure genguard prints a **Summary** (one line per group, with drift kinds), then **Drift** details and diffs. A failed command is still diffed: the group counts as an error, and the summary line names the drift it left behind. A `clean` wipe the command never rewrote is left out of that list. Later groups see the working tree as earlier groups left it. When an earlier group fails after `clean`, paths it wiped and a later group leaves untouched stay out of that later group's drift. A later command that writes those paths is checked as usual. Prefer disjoint `outputs` so a group that drifts cannot change the tree the next group checks.

`genguard check --since origin/main` reruns a group that declares `inputs` when those inputs, its outputs, or the config file differ between the working tree and the merge-base of that ref and `HEAD`. A group with no `inputs` still runs. An unchanged group is `skipped`: its command does not run, and `clean: true` does not delete its outputs. `genguard check --all --since origin/main` uses one merge-base, then the same rule per group. A missing or unrelated ref exits `2` before any group runs.

Exit codes:

A skip does not add a code. A matching run is `0`, including when some groups were skipped. Drift is `1`. A bad `--since` ref, an empty `inputs` list, and a failed command are `2`.

| Code | Meaning |
|---|---|
| `0` | Generated files match |
| `1` | Drift detected |
| `2` | Config, git, or generator command error |

## GitHub Actions (install from release)

[install.sh](../install.sh) picks the archive for the runner (`linux_amd64` on `ubuntu-latest`, `darwin_arm64` on `macos-latest`), checks `checksums.txt`, and installs `genguard` into a directory already on `PATH`. From a checkout of this repository, `sh install.sh` installs `v0.4.0`.

`v0.3.0` and earlier tags do not contain that script. This job downloads the `v0.4.0` archive. Archive names drop the leading `v` (`genguard_0.4.0_linux_amd64.tar.gz`). On Windows the archive is a `.zip` and the binary is `genguard.exe`. Do not point CI at the default branch.

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
          GENGUARD_TAG: v0.4.0
        run: |
          version=${GENGUARD_TAG#v}
          asset="genguard_${version}_linux_amd64.tar.gz"
          curl -fsSL "https://github.com/wuddleko/genguard/releases/download/${GENGUARD_TAG}/${asset}" -o "$asset"
          curl -fsSL "https://github.com/wuddleko/genguard/releases/download/${GENGUARD_TAG}/checksums.txt" -o checksums.txt
          awk -v name="$asset" '$NF == name { print $1 "  " name; found = 1; exit } END { if (!found) exit 1 }' checksums.txt > check.txt
          sha256sum -c check.txt
          tar -xzf "$asset" genguard
          sudo install -m 0755 genguard /usr/local/bin/genguard

      - name: Verify generated files
        run: genguard check
```

`/usr/local/bin` is on `PATH` on GitHub-hosted runners. `install.sh` uses that directory when it is on `PATH` and is writable, or when `sudo` can run without a password. It then tries `/opt/homebrew/bin`, then `~/.local/bin`, when that directory is on `PATH`. If none of those is on `PATH`, it exits with an error. Set `BINDIR` to a directory that is already on `PATH`. The next step cannot see `PATH` changes made inside the script.

## GitHub Actions (Go install)

Fine when the job already uses Go and you want a tagged module version without downloading archives:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Install genguard
        run: go install github.com/wuddleko/genguard/cmd/genguard@v0.4.0

      - name: Verify generated files
        run: genguard check
```

Ensure `$(go env GOPATH)/bin` is on `PATH` (true by default on GitHub-hosted runners after `setup-go`).

## GitLab CI

The image needs `curl`, `awk`, `tar`, and `sha256sum`, plus the generators named in the config. `/usr/local/bin` has to be writable and on `PATH`. Otherwise set `BINDIR` to a directory that is already on `PATH`. A checkout of this repository can run `sh install.sh` instead. That also covers macOS and Windows. The image then needs `curl`, `tar`, `awk`, `mktemp`, and `sha256sum` or `shasum`.

```yaml
genguard:
  script:
    - |
      tag=v0.4.0
      version=${tag#v}
      asset="genguard_${version}_linux_amd64.tar.gz"
      curl -fsSL "https://github.com/wuddleko/genguard/releases/download/${tag}/${asset}" -o "$asset"
      curl -fsSL "https://github.com/wuddleko/genguard/releases/download/${tag}/checksums.txt" -o checksums.txt
      awk -v name="$asset" '$NF == name { print $1 "  " name; found = 1; exit } END { if (!found) exit 1 }' checksums.txt > check.txt
      sha256sum -c check.txt
      tar -xzf "$asset" genguard
      dest=${BINDIR:-/usr/local/bin}
      install -m 0755 genguard "$dest/genguard"
    - genguard check
```

## Monorepo with one config at repo root

```yaml
      - uses: actions/checkout@v4
      - run: go install github.com/wuddleko/genguard/cmd/genguard@v0.4.0
      - run: genguard check
```

Place `genguard.yaml` or `genguard.yml` at the repository root. Paths in `outputs` are relative to that file. A directory holds one of those names; both files make `genguard check` and `genguard check --all` exit `2`.

## Monorepo with config per service

`genguard check --all` discovers every config under the repository root, one `genguard.yaml` or `genguard.yml` per directory. It skips directories named `.git`, `vendor`, and `node_modules`, and it does not read `.gitignore`, so a config inside an ignored directory still runs. A passing run prints each config path and a totals line. A config that skipped a group is printed again with that group's lines. Both names in one directory exit `2`. `genguard check --all --since origin/main` applies `--since` to every group. `--isolated` checks the HEAD copy in a throwaway worktree and does not write the checkout. `--all --isolated` lists configs tracked at HEAD, skips one that is not committed, and still runs a committed config deleted in the checkout. Each config gets its own worktree, so overlapping `clean` outputs do not share a wipe. Groups in one file still share that worktree.

```yaml
      - uses: actions/checkout@v4
      - run: go install github.com/wuddleko/genguard/cmd/genguard@v0.4.0
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

- Source files your generator reads (`.proto`, OpenAPI spec, SQL queries, etc.). List those paths under `inputs` when a pull request should be able to skip the group
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
    inputs:
      - queries/
      - sqlc.yaml
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
git tag v0.4.0
git push origin v0.4.0
```

The [release workflow](../.github/workflows/release.yml) publishes archives for Linux and macOS (`amd64`, `arm64`) and Windows (`amd64`), plus `checksums.txt`. Bump `default_tag` in [install.sh](../install.sh), and the install pins in the README and this file, to the tag you are cutting. That commit has to contain `install.sh` before a raw URL for the tag will serve the script. Write a downloaded `install.sh` to a new file (`script=$(mktemp)`) before running it. Each archive includes the binary, `LICENSE`, `README.md`, `SECURITY.md`, `docs/ci.md`, `genguard.example.yaml`, and `examples/`. Archive filenames use the tag without the `v` (`genguard_0.4.0_linux_amd64.tar.gz`). On Windows the binary is `genguard.exe`; group commands use `sh`/`bash` when present, otherwise `%COMSPEC% /C` (typically `cmd.exe`).
