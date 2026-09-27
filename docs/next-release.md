# Next release

`README.md` and `docs/ci.md` already include the text through the quiet tail, `--verbose`, and `::group::` release. Fold the notes below when this release is cut.

## README

In the config section, name `tools` with the other top-level keys, add the group bullet, and add the section.

Top-level keys are `groups` (required, non-empty), `clean`, and `tools`. An unknown key is an error.

- **`tools`** — optional names from the top-level `tools` list.

### Tools

`tools` declares programs a group can require. Each entry has:

- **`name`** — required, a single token (`git`, `sqlc`).
- **`version`** — optional exact version. A leading `v` is stripped from the pin and from the first version-shaped token in the probe output, and those two texts have to match. `v1.32.0` and `1.32.0` are the same pin.
- **`command`** — optional probe. The default is `name --version`.

A group lists those names under its own `tools` key. A name that is not declared at the top level is a config error. The probes run before `clean` and before the generator. A missing program, or a version that does not match, fails the group and the command does not run. With `--json`, `want` is the pin and `have` is the probed version, both without a leading `v`. When `version` is omitted, `want` is omitted and `have` is the probed version.

## docs/ci.md

Under Tool-specific configs, add this and do not copy the field list:

Pin a generator version with the `tools` key. The fields are in the [README](../README.md#tools). A mismatch fails the group before `clean` and before the generator runs.
