package codex

const AgentsTemplate = `# AGENTS.md

Use goctx before broad Go repository exploration.

Rules:
- Prefer ` + "`goctx task \"<task>\"`" + ` to identify likely packages, files, symbols, and focused tests.
- Use ` + "`goctx symbols <file>`" + ` before reading whole Go files.
- Use ` + "`goctx slice <file> --symbol <name>`" + ` for targeted source context.
- Use ` + "`goctx routes`" + ` for HTTP/API route work.
- Use ` + "`goctx test-summary -- go test ...`" + ` instead of pasting noisy raw test output.
- Avoid generated, vendor, build, dependency lock, and large fixture files unless the task explicitly requires them.
- Do not use git commands.
- Prefer focused tests and minimal, reviewable changes.
`

const SkillTemplate = `---
name: go-token-efficient
description: Use for Go repository work where Codex should reduce token use by using compact package maps, symbol slices, route maps, task packs, and summarized test output before reading full files.
---

# Go Token Efficient Workflow

1. Run ` + "`goctx task \"<task>\"`" + `.
2. Run ` + "`goctx map`" + ` if architecture is unclear.
3. Run ` + "`goctx symbols <file>`" + ` before reading a full file.
4. Run ` + "`goctx slice <file> --symbol <name>`" + ` for the needed symbol body.
5. Run ` + "`goctx routes`" + ` for HTTP/API tasks.
6. Run ` + "`goctx test-summary -- go test ...`" + ` for compact test output.
7. Use full-file reads only if compact context is insufficient.
`

const HooksTemplate = `# GOCTX Hook Guidance

This is guidance, not an active hook configuration.

Recommended guardrails:
- Discourage full-file cat on large Go files until ` + "`goctx symbols`" + ` or ` + "`goctx slice`" + ` has been tried.
- Discourage broad raw search output when ` + "`goctx task`" + ` or ` + "`goctx map`" + ` can narrow context.
- Discourage raw ` + "`go test ./...`" + ` output in chat; prefer ` + "`goctx test-summary -- go test ./...`" + `.
- Discourage reading generated files, vendor directories, build output, dependency locks, and large fixtures.
- Prefer targeted package tests suggested by ` + "`goctx task`" + `.
`

const ConfigTemplate = `# Example Codex config for repositories using goctx.
# This is only an example. Review before copying into any active config.

[goctx]
prefer_task_pack = true
prefer_symbol_slices = true
avoid_generated_files = true
avoid_vendor = true
summarize_tests = true
`

const ScriptTemplate = `#!/usr/bin/env sh
exec goctx "$@"
`
