# Next release

`README.md` and `docs/ci.md` already include the text through the `--isolated` release. Fold the manual below into both files when this release is cut.

## README usage

```bash
genguard run                              # regenerate; writes the checkout
genguard run --since origin/main
genguard run --all
genguard run --all --since origin/main
genguard check --json
genguard check --all --json
genguard run --json
```

## README, after the isolated paragraph

`genguard run` runs the same commands as `genguard check` and leaves the tree as the generator wrote it. It does not fail when that differs from HEAD. A skip under `--since` does not run `clean`. `--isolated` is not valid: run writes the checkout. `genguard run --all` runs every config in path order on that same checkout, so a `clean` in one file still deletes paths a later config is about to use.

`--json` prints one JSON object on stdout and does not print the human report. The object has `exit` and `configs`. Each config has `path`, `exit`, and `groups`, and `error` when loading that config failed or an isolated worktree could not be removed. `path` is relative to the repository root for one config and for `--all`. A group has `name`, `status` (`ok`, `drift`, `error`, `skipped`), and, when present, `error` and `drifts` (`kind`, `path`). A drift `path` is relative to the repository root. Diffs stay in the human report. Exit codes are unchanged. A non-empty `error` does not replace `exit`. A failure before any config runs still prints `error:` on stderr and leaves stdout empty.

## `docs/ci.md`, GitHub Action

```yaml
- uses: actions/checkout@v4
  with:
    fetch-depth: 0
- uses: wuddleko/genguard@v0.5.0
  with:
    all: true
    since: origin/main
```

The action installs the release named by `uses:` and runs `genguard check`. It does not check out the repo, install generators, or commit. Inputs are `all`, `config`, `since`, and `isolated`. `--since` needs that ref in the checkout. `actions/checkout` fetches one commit unless `fetch-depth` is `0`. Drifted paths are `::error` annotations, relative to `GITHUB_WORKSPACE`. `GENGUARD_ANNOTATIONS=false` skips those lines. A `uses:` value of `v1.2.3` installs that release. `v1` and `v1.2` are rejected. `uses: ./`, a branch, and a commit SHA build the action checkout with `go install`.
