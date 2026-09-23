# Next release

`README.md` and `docs/ci.md` already include the text through the `--since` release. Fold the manual below into both files when this release is cut.

## README usage

```bash
genguard check --isolated                 # HEAD copy; leaves the checkout alone
genguard check --all --isolated
```

## README, after the `--all` paragraph

`genguard check --isolated` checks the committed copy of the config in a throwaway worktree and does not read or write dirty files in the checkout. Groups in that file still share the worktree. `--all --isolated` finds configs tracked at HEAD, including one deleted in the checkout, and skips a config that is not committed. Each config gets its own full worktree, one after another, so a failed `clean` in one file does not wipe files another config is judging.

## README, end of "More than one group"

`--all --isolated` gives each config its own tree. Groups in one file still share that tree.

## `docs/ci.md`, end of the monorepo paragraph

`--isolated` checks the HEAD copy in a throwaway worktree and does not write the checkout. `--all --isolated` lists configs tracked at HEAD, skips one that is not committed, and still runs a committed config deleted in the checkout. Each config gets its own worktree, so overlapping `clean` outputs do not share a wipe. Groups in one file still share that worktree.
