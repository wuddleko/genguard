# Examples

Copy a template into your repo as `genguard.yaml` (or `genguard.yml`) and adjust paths to match your layout.

These files are **config only**. They point at tools you already use (`buf`, `sqlc`, `go generate`, etc.). genguard does not ship generators — it re-runs whatever `command` declares and fails CI if git drifted.

| Template | Typical stack | Command |
|---|---|---|
| [go-generate.yaml](go-generate.yaml) | Go `//go:generate` | `go generate ./...` |
| [buf.yaml](buf.yaml) | Protobuf | `buf generate` |
| [openapi.yaml](openapi.yaml) | OpenAPI clients/types | `openapi-generator-cli generate ...` |
| [sqlc.yaml](sqlc.yaml) | SQL → Go | `sqlc generate` |
| [make.yaml](make.yaml) | Makefile-driven codegen | `make generate` |

## Usage

```bash
cp examples/buf.yaml path/to/your/service/genguard.yaml
# edit outputs: and paths if needed
genguard check --config path/to/your/service/genguard.yaml
```

Or place `genguard.yaml` beside the generated-files directory (not inside it) and run from a git work tree:

```bash
cd path/to/your/service
genguard check
```

`genguard check -c path/to/genguard.yaml` is the same as `--config`.

## What you need in your repo

1. **`genguard.yaml` or `genguard.yml`** — from one of these templates
2. **Source files** — `.proto`, `.sql`, OpenAPI spec, etc. (whatever your tool reads)
3. **Generated output** — committed to git under the paths listed in `outputs` (not gitignored)
4. **The tool on PATH in CI** — same as when you run codegen locally

## CI

See [docs/ci.md](../docs/ci.md) for GitHub Actions snippets that install `genguard` and run `genguard check`.
