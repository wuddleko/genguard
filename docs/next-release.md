# Next release

Manual text for the release after v0.2.1. `README.md` and `docs/ci.md` stay as published until that release. Fold the sections below into both files then. The exit-code row does not change.

## After the command

A failed command is still diffed. The group counts as an error, so the run exits 2. The Drift section lists whatever that command left behind.

A `clean: true` wipe that the command never rewrote is left out of that list. The error already says the outputs are gone. A file the failed command rewrote is listed.

The summary line names both facts. The totals line counts the group once, as an error:

```
Summary
  protobuf: error (command failed (exit 1): buf generate failed); drift (1 modified)
1 group: 0 ok, 0 drift, 1 error

Drift
[modified] protobuf: gen/foo.pb.go

diff --git a/gen/foo.pb.go ...

error: command failed (exit 1): buf generate failed
```

When this is the only failure, the final line stays the command error. When another group drifted on its own, the final line is `error: 1 group failed; 1 group drifted`.

## `inputs` and `--since`

A group can name the sources that feed its command. Paths are git pathspecs, relative to the config file, same as `outputs`.

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

Leave `inputs` off and the group runs on every check. An empty `inputs` list is a config error, and so is an unknown key.

`genguard check --since origin/main` resolves the merge-base of that ref and `HEAD`. A group with `inputs` runs when the working tree differs from that merge-base, or from `HEAD`, under its inputs, its outputs, or its config file (`genguard.yaml` or `genguard.yml`). A committed change on the branch counts. So does an unstaged edit, a staged edit, or an untracked file that is not gitignored. A hand-edit of a generated file still runs that group. So does a change to the group's command or `clean`, a new untracked config, and a declared output file that is not on disk. Directory outputs and globs are not that check. A group with no `inputs` still runs.

An unchanged group is `skipped`. Its command does not run, and `clean: true` does not delete its outputs. It is not counted as ok. A passing check prints the skips after the success sentence:

```
Generated files match the generators.
  sqlc: OK
  protobuf: skipped
2 groups: 1 ok, 0 drift, 0 error, 1 skipped
```

Without `--since`, every group runs and `inputs` is ignored.

`genguard check --all --since origin/main` uses one merge-base for the repository, then the same rule per group. One config can run sqlc and skip protobuf. A passing `--all` run still prints each config path and the config totals. Each config that skipped a group is printed again, with that config's group lines:

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

## Exit codes

A skip does not add a code. A matching run is `0`, including when some groups were skipped. Drift is `1`. A bad `--since` ref, an empty `inputs` list, and a failed command are `2`.

| Exit code | Meaning |
|---|---|
| `0` | Generated files match |
| `1` | Drift |
| `2` | Config, git, or the generator command failed. A command error wins over drift |
