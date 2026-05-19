# goctx

goctx is a Go repository context optimizer for Codex. It analyzes normal Go modules and returns compact package, symbol, route, task, test, diff, and token-risk context so Codex can inspect less noisy source before editing.

goctx does not change model tokenization directly. It reduces context waste by returning smaller, more relevant information before broad file reads.

## Build

```sh
go build ./cmd/goctx
```

Optional install from this repository:

```sh
go install ./cmd/goctx
```

## Commands

```sh
goctx help
goctx scan
goctx map
goctx symbols internal/auth/firebase.go
goctx slice internal/auth/firebase.go --symbol VerifyToken
goctx task "fix Firebase JWT auth middleware"
goctx routes
goctx test-summary -- go test ./internal/auth -run TestVerifyToken
goctx diff-summary
goctx token-report
goctx init-codex
```

## Codex Workflow

Use goctx before broad exploration:

```sh
goctx task "fix Firebase JWT auth middleware"
goctx symbols internal/auth/firebase.go
goctx slice internal/auth/firebase.go --symbol VerifyToken
goctx routes
goctx test-summary -- go test ./internal/auth -run TestVerifyToken
goctx token-report
```

`goctx init-codex` generates conservative integration files:

- `AGENTS.md`
- `.agents/skills/go-token-efficient/SKILL.md`
- `.agents/skills/go-token-efficient/scripts/goctx`
- `.codex/GOCTX_HOOKS.md`
- `.codex/config.example.toml`

Existing files are skipped unless `--force` is passed.

## What It Does

- Scans repositories and classifies source, tests, generated files, vendor/build output, migrations, fixtures, docs, and unknown files.
- Uses `golang.org/x/tools/go/packages` for package loading.
- Uses `go/parser`, `go/ast`, `go/token`, and related standard packages for symbols, slicing, and route extraction.
- Slices Go source by function, method, type, interface, const, var, or test symbol.
- Produces heuristic task context packs without embeddings or external AI calls.
- Extracts likely Echo, Gin, Chi, and `net/http` route registrations.
- Runs only direct `go` commands for `test-summary`; it does not invoke a shell.
- Estimates token risk with `size_bytes / 4`.
- Summarizes changed files only from `.goctx/baseline.json` when present; it does not use VCS commands.

## Safety Notes

- goctx is safe to run from a Go repository root.
- It does not modify source code.
- `init-codex` is the only MVP command that writes files.
- Generated, vendor, build, dependency lock, and large fixture files are avoided by default.
- `test-summary` only accepts commands whose first argument is `go`.
- The CLI does not use git commands.

## MVP Limitations

- Ranking is heuristic keyword matching, not semantic search.
- Route extraction covers common selector-call patterns and may miss custom wrappers.
- Symbol slicing is file-local and AST-span based.
- Token estimates are rough byte-count estimates, not exact tokenizer counts.
- `diff-summary` requires `.goctx/baseline.json`; snapshot creation is deferred.
- Package loading depends on the target module being loadable by Go tooling.

## Deferred V2 Items

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
