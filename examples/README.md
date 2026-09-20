# Examples

Copy a template into your repo as `regen.yaml` (or `regen.yml`) and adjust paths to match your layout.

These files are **config only**. They point at tools you already use (`buf`, `sqlc`, `go generate`, etc.). regen does not ship generators — it re-runs whatever `command` declares and fails CI if git drifted.

| Template | Typical stack | Command |
|---|---|---|
| [go-generate.yaml](go-generate.yaml) | Go `//go:generate` | `go generate ./...` |
| [buf.yaml](buf.yaml) | Protobuf | `buf generate` |
| [openapi.yaml](openapi.yaml) | OpenAPI clients/types | `openapi-generator-cli generate ...` |
| [sqlc.yaml](sqlc.yaml) | SQL → Go | `sqlc generate` |
| [make.yaml](make.yaml) | Makefile-driven codegen | `make generate` |

## Usage

```bash
cp examples/buf.yaml path/to/your/service/regen.yaml
# edit outputs: and paths if needed
regen check --config path/to/your/service/regen.yaml
```

Or place `regen.yaml` beside the generated-files directory (not inside it) and run from a git work tree:

```bash
cd path/to/your/service
regen check
```

`regen check -c path/to/regen.yaml` is the same as `--config`.

## What you need in your repo

1. **`regen.yaml` or `regen.yml`** — from one of these templates
2. **Source files** — `.proto`, `.sql`, OpenAPI spec, etc. (whatever your tool reads)
3. **Generated output** — committed to git under the paths listed in `outputs` (not gitignored)
4. **The tool on PATH in CI** — same as when you run codegen locally

## CI

See [docs/ci.md](../docs/ci.md) for GitHub Actions snippets that install `regen` and run `regen check`.
