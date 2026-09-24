# Next release

`README.md` and `docs/ci.md` already include the text through the `--isolated` release. Fold the manual below into both files when this release is cut.

## README usage

```bash
genguard run                              # regenerate; writes the checkout
genguard run --since origin/main
```

## README, after the isolated paragraph

`genguard run` runs the same commands as `genguard check` and leaves the tree as the generator wrote it. It does not fail when that differs from HEAD. A skip under `--since` does not run `clean`. `--isolated` is not valid: run writes the checkout.
