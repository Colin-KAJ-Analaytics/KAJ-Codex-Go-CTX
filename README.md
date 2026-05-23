# goctx

goctx builds compact, budgeted Go context packs for Codex so agents can inspect fewer files, read smaller symbols, run narrower tests, and avoid generated or noisy source.

goctx does not change tokenizer behavior. Its savings reports are estimated context reductions based on rough local byte/token estimates, not exact tokenizer or billing math.

## Build

```sh
go build ./cmd/goctx
```

Optional install from this repository:

```sh
go install ./cmd/goctx
```

## Core Commands

```sh
goctx help
goctx discover
goctx doctor
goctx scan
goctx map
goctx codex-pack "fix auth middleware" --budget 2000
goctx codex-pack "add validation to create lead" --format json
goctx task "fix Firebase JWT auth middleware"
goctx symbols internal/auth/firebase.go
goctx slice internal/auth/firebase.go --symbol VerifyToken
goctx routes
goctx smells --budget 1500
goctx test-summary -- go test ./internal/auth -run TestVerifyToken
goctx baseline create
goctx diff-summary
goctx token-report
goctx init-codex --strict
```

## Public Repo Workflow

For arbitrary public Go repositories, start with:

```sh
goctx discover
goctx doctor
goctx codex-pack "describe the task" --budget 2000
```

Then follow the read plan:

```sh
goctx symbols path/to/file.go
goctx slice path/to/file.go --symbol SymbolName
goctx test-summary -- go test ./focused/package
```

goctx does not assume `cmd/`, `internal/`, `handlers/`, `services/`, `repositories/`, or any preferred architecture. It discovers Go modules, nested modules, `go.work` workspaces, likely main packages, imports, symbols, routes, tests, and token-risk files from the repository it is run against.

## Budgets And Formats

Context-producing commands support budget and output flags:

```sh
goctx codex-pack "fix auth middleware" --budget 1200 --max-files 8 --max-symbols 20
goctx task "fix route panic" --format json
goctx routes --format markdown --max-routes 20
goctx smells --format json --budget 800
```

Supported formats:

- `text`
- `json`
- `markdown`

JSON output uses a stable envelope with fields such as `command`, `root`, `module`, `budget_estimated_tokens`, `actual_estimated_tokens`, `warnings`, `omitted`, `files`, `symbols`, `routes`, `tests`, `read_plan`, and `confidence`.

## Fallback Indexing

goctx first tries `golang.org/x/tools/go/packages`. If package loading is incomplete or broken because of private dependencies, missing generated files, invalid environments, or incomplete modules, it degrades to AST-only indexing and scanner-backed output.

Fallback output includes warnings and a confidence level:

- `high`: package loading was useful
- `medium`: fallback or partial package data
- `low`: scanner-only or very limited context

## Ignore Configuration

Defaults avoid generated, vendor, build, dependency lock, and fixture-heavy files.

Optional `.goctx.yaml`:

```yaml
ignore:
  dirs:
    - vendor
    - node_modules
    - dist
    - build
    - coverage
    - tmp
    - bin
    - .next
    - .cache
    - .git
    - snapshots
    - golden
    - generated
  files:
    - "*.pb.go"
    - "*.gen.go"
    - "*_generated.go"
    - "wire_gen.go"
    - "mock_*.go"
  max_file_bytes: 262144
  max_fixture_bytes: 65536
```

Optional `.goctxignore` supports simple newline patterns:

```text
# comments and blank lines are ignored
snapshots/
*.golden
generated/
```

Simple `.gitignore` patterns are also treated as additive ignore hints. goctx does not use git commands.

## Baselines And Diff Summary

goctx can create a filesystem baseline without VCS:

```sh
goctx baseline create
goctx baseline status
goctx diff-summary
goctx baseline update
```

The baseline is stored at:

```text
.goctx/baseline.json
```

It contains file paths, sizes, hashes, classifications, timestamp, and root metadata for non-ignored files.

## Codex Integration

`goctx init-codex --strict` generates conservative generic guidance:

- `AGENTS.md`
- `.agents/skills/go-token-efficient/SKILL.md`
- `.agents/skills/go-token-efficient/scripts/goctx`
- `.codex/GOCTX_HOOKS.md`
- `.codex/config.example.toml`

Existing files are skipped unless `--force` is passed:

```sh
goctx init-codex --strict --force
```

The strict guidance tells Codex to start Go tasks with `goctx codex-pack` or `goctx task`, use symbols and slices before broad full-file reads, summarize tests through `goctx test-summary`, and avoid generated/noisy files.

## Safety Notes

- goctx is safe to run from a repository root or from a directory above/below a Go module.
- It does not modify source code.
- File-writing commands are explicit: `init-codex` and `baseline create/update`.
- `test-summary` only executes commands whose first argument is `go`.
- No git commands are used.
- No embeddings, external AI calls, vector databases, MCP server, web UI, or daemon mode are included.

## Limitations

- Token estimates use rough byte-count heuristics.
- Savings reports are estimated context reductions only.
- Route extraction covers common direct calls and likely wrappers; custom frameworks may need manual inspection.
- AST fallback is useful but less precise than full package loading.
- Ignore parsing intentionally supports a small safe subset, not complete `.gitignore` semantics.
- Smell detection focuses on Codex navigation/token risk, not subjective style review.

## Deferred Items

- embeddings
- semantic vector search
- MCP server
- web UI
- daemon mode
- full SSA call graph
- OpenAPI generation
- Supabase-specific inspection
- Firebase-specific inspection
- cross-language support
