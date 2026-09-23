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

## Exit codes

| Exit code | Meaning |
|---|---|
| `0` | Generated files match |
| `1` | Drift |
| `2` | Config, git, or the generator command failed. A command error wins over drift |
