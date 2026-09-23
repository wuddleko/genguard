# genguard

`genguard check` runs your code generators and fails if the result doesn't match the files already in git. A hand-edited `.pb.go` or a schema change nobody regenerated shows up as a diff. With `clean: true`, genguard deletes the declared outputs first, so a file the generator stopped writing fails the check too.

The command can be anything you can run in a shell. `buf generate`, `sqlc generate`, `go generate`, a Make target, your own script.

## Run it

Put a `genguard.yaml` (or `genguard.yml`) next to the generated files, outside the output directory. Templates for the usual tools are in [examples/](examples/README.md).

```yaml
groups:
  - name: protobuf
    command: buf generate
    outputs:
      - gen/
```

Then, from a git checkout:

```bash
genguard check
```

A match prints `Generated files match the generators.` and exits 0. Drift prints a summary, the paths, and a diff, then exits 1:

```
Summary
  protobuf: drift (1 modified)
1 group: 0 ok, 1 drift, 0 error

Drift
[modified] protobuf: gen/foo.pb.go

diff --git a/gen/foo.pb.go ...

error: 1 generated path drifted; commit the generator output or fix the command
```

Regenerate and commit. genguard leaves the tree as the generator wrote it and prints the diff. A generated file that is only staged still fails until that commit exists.

```bash
genguard check -c path/to/genguard.yaml   # same as --config
genguard check --all                      # every config in the repo
genguard version
```

The diff is against `HEAD` in the checkout you just made. On a pull request that is the PR branch tip. genguard is asking whether this commit's outputs still match this commit's inputs.

## Install

Download an archive from [GitHub Releases](https://github.com/wuddleko/genguard/releases). The filename drops the leading `v`; the URL keeps the tag. Each release also has `checksums.txt`.

```bash
GENGUARD_TAG=v0.2.0
GENGUARD_VERSION=${GENGUARD_TAG#v}
curl -fsSL \
  "https://github.com/wuddleko/genguard/releases/download/${GENGUARD_TAG}/genguard_${GENGUARD_VERSION}_linux_amd64.tar.gz" \
  -o genguard.tar.gz
tar xzf genguard.tar.gz genguard
sudo install genguard /usr/local/bin/genguard
```

Change the archive name for the machine (`darwin_arm64`, `linux_amd64`, `windows_amd64.zip`, and so on). On Windows the binary is `genguard.exe`.

From source, with Go 1.22+:

```bash
make install
# or, from this repository
go install ./cmd/genguard
# or a published module version
go install github.com/wuddleko/genguard/cmd/genguard@v0.2.0
```

`$(go env GOPATH)/bin` has to be on your `PATH`.

## Config

With no `--config`, genguard walks up from the current directory until it finds `genguard.yaml` or `genguard.yml`. That file has to sit inside a git work tree. A directory gets one of those names. Both files in the same directory make `genguard check` and `genguard check --all` exit 2. `--config` still checks the path you gave it.

`genguard check --all` runs every config under the repository root, in path order, on the same working tree. It skips directories named `.git`, `vendor`, and `node_modules`. It does not read `.gitignore`, so a config inside an ignored directory still runs. A clean run prints each config path and a totals line. `--all` and `--config` are separate invocations.

A group has:

- **`name`** — label in the output. Optional. The fallback is `groups[0]`, `groups[1]`, and so on.
- **`command`** — the shell command. genguard runs it with `sh -c` from the directory that contains the config, and it will run whatever that command is. On Windows it uses `sh` or `bash` when one of those is on `PATH` (Git Bash); otherwise it uses `cmd.exe`. A POSIX recipe needs a POSIX shell.
- **`outputs`** — git pathspecs, relative to the config file, for the files to compare.
- **`clean`** — optional, default `false`. Delete those outputs before the command runs. You can also set `clean` once at the top of the file and every group inherits it.

After the command, genguard compares `HEAD` to the working tree under `outputs`:

| What you see | What happened |
|---|---|
| `modified` | A tracked file is still there, and it changed |
| `untracked` | The generator wrote a file git does not know about, and the file is not gitignored |
| `missing` | A tracked file under `outputs` is gone, or a **file** you listed is not there after the command. A directory or a glob (`gen/`, `*.go`) is not, by itself, a missing path |

Generated paths have to be tracked. Gitignored files under `outputs` are left out of the report.

### `clean`

Turn this on when a generator might stop emitting a file. Without it, `old.go` just sits there and a diff never mentions it. With `clean: true`, genguard deletes the declared outputs, runs the command, and reports `old.go` as missing if it did not come back.

The delete is real.

- A directory output is removed and recreated empty.
- A file output is removed.
- A glob removes the tracked and untracked files git matches, and leaves ignored files alone. `*_queries.sql.go` can live next to Go you wrote by hand.

It will refuse `.`, `..`, absolute paths, anything that escapes the config directory, symlinks, and any delete that would take `.git` or the config file with it. Every output is checked before anything is removed, so one refused path leaves the tree as it was. If the command fails after a wipe, the error says the outputs are already gone. They are not put back.

Starting points: [genguard.example.yaml](genguard.example.yaml) and the templates in [examples/](examples/README.md) (`go generate`, buf, OpenAPI, sqlc, Make). Those files are config only. The generator stays yours.

## More than one group

Groups run in the order you listed them, and every group runs. Drift or a command error in the first one still lets the rest go. You get a Summary line per group, then the Drift paths and diffs.

Each group sees the working tree the previous group left behind. Give them outputs that don't overlap, or a drift in one group changes what the next group is judging. `genguard check --all` follows the same rule across config files.

If an earlier group has `clean: true` and then the command fails, paths it wiped and a later group never writes stay out of that later group's drift. A later command that does write them is checked as usual.

| Exit code | Meaning |
|---|---|
| `0` | Generated files match |
| `1` | Drift |
| `2` | Config, git, or the generator command failed. A command error wins over drift |

## Security

What a run is allowed to do, and how to report a vulnerability: [SECURITY.md](SECURITY.md).

## CI

GitHub Actions snippets, monorepo layouts, and per-tool setup are in [docs/ci.md](docs/ci.md).

## Development

```bash
make test
make lint   # go vet ./...
```

Tests need `git` and `python3`. The fixtures use `python3` as a stand-in generator.

## License

MIT. See [LICENSE](LICENSE).
