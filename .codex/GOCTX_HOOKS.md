# GOCTX Hook Guidance

This is guidance, not an active hook configuration.

Recommended guardrails:
- Discourage full-file cat on large Go files until `goctx symbols` or `goctx slice` has been tried.
- Discourage broad raw search output when `goctx task` or `goctx map` can narrow context.
- Discourage raw `go test ./...` output in chat; prefer `goctx test-summary -- go test ./...`.
- Discourage reading generated files, vendor directories, build output, dependency locks, and large fixtures.
- Prefer targeted package tests suggested by `goctx task`.
