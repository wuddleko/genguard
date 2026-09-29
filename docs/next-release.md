# Next release

`README.md` and `docs/ci.md` already include the text through the quiet tail, `--verbose`, and `::group::` release. Fold the notes below when this release is cut.

## README

In the config section, name `tools` and `timeout` with the other top-level keys, add the group bullets, and add the Tools section.

Top-level keys are `groups` (required, non-empty), `clean`, `tools`, and `timeout`. An unknown key is an error.

- **`tools`** — optional names from the top-level `tools` list.
- **`timeout`** — optional Go duration greater than zero (`200ms`, `1m`). A top-level value applies to every group. A group value replaces it. Omitting it on a group keeps the top-level value. Probes use that same limit. When the key is absent, the command has no limit. `0s`, a negative duration, a non-string, and a string that is not a Go duration are errors.

### Tools

`tools` declares programs a group can require. Each entry has:

- **`name`** — required, a single token (`git`, `sqlc`).
- **`version`** — optional exact version. A leading `v` is stripped from the pin and from the first version-shaped token in the probe output, and those two texts have to match. `v1.32.0` and `1.32.0` are the same pin.
- **`command`** — optional probe. The default is `name --version`.

A group lists those names under its own `tools` key. A name that is not declared at the top level is a config error. The probes run before `clean` and before the generator. A missing program, or a version that does not match, fails the group and the command does not run. With `--json`, `want` is the pin and `have` is the probed version, both without a leading `v`. When `version` is omitted, `want` is omitted and `have` is the probed version.

## docs/ci.md

Under Tool-specific configs, add this and do not copy the field list:

Pin a generator version with the `tools` key. The fields are in the [README](../README.md#tools). A mismatch fails the group before `clean` and before the generator runs.

## Null and empty

A missing key stays the default. A present null and a present empty value take the same branch.

- A tool `version` or `command` that is null, `""`, or whitespace requires a non-empty string. Omitting the key leaves it unset.
- `clean: ~` is a boolean error. `timeout: ~` is a duration error. Omitting either key leaves it unset.
- A null or blank entry in `outputs` or `inputs` must be a non-empty string.
- A group `name` that is null, empty, or whitespace uses `groups[0]`, `groups[1]`, and so on.

## Config discovery

`check --all` lists configs with `git ls-files`: the index, plus untracked files that are not ignored. `check --all --isolated` lists HEAD with `git ls-tree`, so a staged config is absent and a committed config removed from the index is still listed. A gitignored untracked config is in neither list. A config under `vendor` or `node_modules` is skipped. If git cannot open a directory that can hold a config, discovery fails.

## Overlapping outputs

Two output specs that can name the same path are a load error. A trailing slash is the whole directory. A literal overlaps a glob only when that literal matches the glob, and two globs overlap only when one path could match both. Within one file the error names both groups and both specs. `check --all`, `run --all`, and `check --all --isolated` use the same check across configs and name both config paths. Isolated reads the committed files, including a config removed from the checkout. A file that does not parse is reported for that config. Outputs from a file that parses still count, so another config that names one of those paths does not run. The commands do not run, so neither path is reported as drift.

## Interrupts

An interrupt before any group has run prints `error: interrupted` and exits 2. After that, the Summary covers the groups that ran and the final line adds `error: interrupted`, including when the earlier groups all passed or one had already failed.
