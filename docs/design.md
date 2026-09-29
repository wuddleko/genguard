# Design

genguard re-runs the commands in `genguard.yaml` and fails when declared outputs do not match `HEAD`. This note is the internals. The user-facing contract is the [README](../README.md).

## Invariants

- **Git decides drift.** A path is `modified`, `untracked`, or `missing` because `git diff HEAD` and `git ls-files --others --exclude-standard` say so, plus a listed file spec that is absent. Gitignored files under `outputs` are not reported.
- **genguard does not own generators.** No plugin API. The command is `sh -c` from the config directory. Tool probes can fail a missing binary or a version pin; they do not install anything.
- **`check` does not commit and does not restore a failed wipe.** After a run, the working tree is whatever the commands wrote. `--isolated` is the way to leave the checkout alone.
- **Groups share a tree.** They run in order, including after an earlier failure. Overlapping `outputs` are a load error, in one file and across `--all`.
- **Exit 1 is drift. Exit 2 is “the check could not run.”** A command error wins over drift.

`command` is a shell as you. `clean` is the one deletion genguard performs itself, so that path is guarded: no `.git`, no config file, no symlink, nothing outside the config directory.

## Layout

```
cmd/genguard            version stamp, os.Exit
internal/cli            flags, config lookup, output, Actions wrapping
internal/config         YAML, FindConfig, output specs, overlap
internal/discover       --all listing, HEAD copies of configs
internal/check          skip, tools, clean, run, drift, reports, JSON
internal/check/clean    wipe with guards
internal/command        sh -c, timeout, process group
internal/gitx           git -C <root>
internal/pathx          RelInside, glob detection
internal/actions        ::group::, ::error, stop-commands
```

`check` and `run` share the group loop. `run` stops after the command. `check` then diffs against `HEAD`. `--isolated` is `check` only.

`Execute` runs one config and `ExecuteAll` every config. Each resolves the repository root and the `--since` merge-base once, and returns a `RunResult` of `ConfigRun` values. Each loaded file is a `ConfigResult` of `GroupResult` values (`ok`, `drift`, `error`, `skipped`). `RunResult.All` selects the `--all` report.

Git is a subprocess, not go-git. Drift then uses the same ignore rules and pathspecs as the repo’s Git.

## Pipeline

Every group, in order:

```
canceled?  → stop; with no group run yet this is an error,
             otherwise the result notes the interrupt

--since and inputs declared and unchanged
  vs merge-base and HEAD, no missing literal output
  → skipped (no probe, no clean, no command)

tools[]    → probe; mismatch is error, before clean
clean?     → wipe; snapshot the wipe (“clean damage”)
command    → on failure: error, still diff, omit paths equal to the snapshot
ModeCheck  → drift vs HEAD → ok | drift
```

A failed command is still diffed so the summary can name both the crash and the files it left behind.

### Drift

| Kind | Meaning |
|---|---|
| `modified` | A tracked file differs from `HEAD` |
| `untracked` | A new file that is not gitignored |
| `missing` | A tracked file under `outputs` is gone, or a listed **file** spec is absent. A directory or a glob is not, by itself, missing |

`git diff` is `-c diff.relative=false --no-renames --name-only -z` against `HEAD`, pathspecs limited to that group’s `outputs`. Untracked files are `git ls-files --others --exclude-standard --full-name`. Drift paths are relative to the repository root in the result, the report, JSON, annotations, and the `git diff` that shows them.

After `clean`, paths that still match the post-wipe snapshot are omitted from that group’s drift: they are the wipe, not a rewrite. A later group does not report paths an earlier failed group deleted and this group never touched.

### Overlap

Two output specs that can name the same path fail at load. A trailing slash is the whole directory. A literal overlaps a glob only when that literal matches the glob, and two globs overlap only when one path could match both. `--all` uses the same check across config files. The commands do not run.

## `--since`

`genguard check --since origin/main` resolves the ref (`rev-parse --verify --end-of-options <ref>^{commit}`), then `merge-base HEAD <rev>`. A group with `inputs` runs if:

- a literal (non-glob, non-directory) output file is absent, or
- `git diff` against that merge-base *or* against `HEAD` names any of inputs, outputs, or the config file, or
- any of those specs has an untracked file

A group with no `inputs` always runs. `--all --since` computes the merge-base once at the repo root. A missing or unrelated ref exits 2 before any group runs. A skip does not run the command and does not `clean`.

This is not a cache of generator output. It is “has anything this group claims to care about changed?”

## Isolated mode

`--isolated` checks the committed tree and does not write the caller’s checkout.

1. Resolve `--since` to its merge-base in the caller’s repo (refs like `@{u}` are meaningless in a detached worktree).
2. Create a temp directory, mode `0700`.
3. `git worktree add --detach <dir> HEAD` with `core.hooksPath` pointed at a missing directory, so `post-checkout` cannot edit the new tree.
4. Map the config path into that worktree, load, run the same check.
5. `worktree remove --force` and delete the parent temp dir.

`--all --isolated` gives each config its own worktree. Groups in one file still share that worktree. A config not in HEAD fails with “not in HEAD.” Paths in errors are rewritten back to the caller’s spelling. `run --isolated` is invalid.

Discovery differs from a normal `--all`:

| State | `check --all` | `--all --isolated` |
|---|---|---|
| Committed config | listed (`ls-files`) | listed (`ls-tree HEAD`) |
| Staged-only config | listed | absent |
| Committed, deleted in the checkout | absent | listed |
| Untracked, not ignored | listed | absent |
| Gitignored untracked | absent | absent |
| Under `vendor/` or `node_modules/` | skipped | skipped |

Without `--all`, config discovery walks from cwd up to the work tree root for `genguard.yaml` or `genguard.yml`. One of those names per directory; both is exit 2.

## Clean

`clean: true` is the only deletion genguard performs. Specs are planned, then guarded, then removed.

Refused: `.`, `..`, absolute paths, paths that escape the config directory, a symlink anywhere in the path or as the target, anything that would remove `.git` or the config file (including a `.git` nested under an output directory).

Then: a directory spec is emptied and left as an empty directory; a file spec is unlinked; a glob is expanded with `git ls-files` (tracked) and `ls-files --others --exclude-standard` (untracked). A glob that matches a directory is an error.

On Unix the remove pins the path component by component so a concurrent rename cannot swing the delete onto another tree. A failed command does not restore the wipe.

## Commands

Unix: `sh -c`. Windows: `sh` or Git’s `bash.exe` if present, otherwise `%COMSPEC% /C`. WSL’s `system32\bash.exe` is skipped.

Stdout and stderr share a 50-line, 8KiB-per-line ring. A successful command is quiet. A failure prints that tail, labeled with the group name (`greeting:`), or `path/to/genguard.yaml: greeting:` under `--all`. `--verbose` streams each line as it arrives.

Timeout is a Go duration on the group or at the top level. The same limit applies to tool probes. No key means no limit.

Interrupt (`SIGINT` / `SIGTERM`) and timeout kill the process group (`Setpgid`, `kill(-pid, SIGKILL)`). Darwin also walks `kern.proc.all` and kills descendants that left the group. The CLI restores the default signal action after the first signal so a second Ctrl-C kills the process normally.

Top-level `tools` declare `{name, version?, command?}`. The default probe is `name --version`. A leading `v` is stripped from the pin and from the first version-shaped token in the probe output. A group may only name tools declared at the top level. The groups of one config share a probe with the same command, pin, and timeout. Probes run in the config directory, so configs do not share them.

Unknown YAML keys are errors. A present null is not “use the default”: `timeout: ~` is a duration error; omitting the key leaves it unset. Pathspecs starting with `:` are rejected so Git magic such as `:(exclude)` cannot be treated as a clean path.

## Results

Human failure output: command tails, a **Summary** (one line per group, then counts), **Drift** lines plus `git diff HEAD` (or `--no-index` against `/dev/null` for untracked files), a final `error:` line.

`--json` prints `exit`, `configs`, `groups`, `drifts`, `tools` on stdout and keeps tails on stderr.

A skipped group contributes nothing to the exit code. The run exit is the max of group exits; a 2 anywhere wins over a 1.

On `GITHUB_ACTIONS=true`, each group that runs is a `::group::`. Generator output is wrapped in `::stop-commands::<token>` so a `::` in the log cannot inject workflow commands. Drifted paths are `::error` annotations relative to `GITHUB_WORKSPACE`. `GENGUARD_ANNOTATIONS=false` skips annotations and still opens groups.

## Security

A vulnerability is genguard doing something the config did not ask for. Running `command`, deleting listed `outputs` when `clean` is set, and printing a diff of those paths are the job. See [SECURITY.md](../SECURITY.md).
