---
name: go-token-efficient
description: Use for Go repository work where Codex should reduce token use by using compact package maps, symbol slices, route maps, task packs, and summarized test output before reading full files.
---

# Go Token Efficient Workflow

1. Run `goctx task "<task>"`.
2. Run `goctx map` if architecture is unclear.
3. Run `goctx symbols <file>` before reading a full file.
4. Run `goctx slice <file> --symbol <name>` for the needed symbol body.
5. Run `goctx routes` for HTTP/API tasks.
6. Run `goctx test-summary -- go test ...` for compact test output.
7. Use full-file reads only if compact context is insufficient.
