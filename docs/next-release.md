# Next release

`README.md` and `docs/ci.md` already include the text through the `--isolated` release. Fold the manual below into both files when this release is cut.

## README usage

```bash
genguard run                              # regenerate; writes the checkout
genguard run --since origin/main
genguard run --all
genguard run --all --since origin/main
```

## README, after the isolated paragraph

`genguard run` runs the same commands as `genguard check` and leaves the tree as the generator wrote it. It does not fail when that differs from HEAD. A skip under `--since` does not run `clean`. `--isolated` is not valid: run writes the checkout. `genguard run --all` runs every config in path order on that same checkout, so a `clean` in one file still deletes paths a later config is about to use.
