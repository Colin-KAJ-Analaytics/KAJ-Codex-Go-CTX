# AGENTS.md

Use goctx before broad Go repository exploration.

Rules:
- Prefer `goctx task "<task>" --agent` to identify the edit target and focused tests. Use the broader task pack if the result needs more context.
- Use `goctx symbols <file>` before reading whole Go files.
- Use `goctx slice <file> --symbol <name>` for targeted source context.
- Use `goctx routes` for HTTP/API route work.
- Use `goctx test-summary -- go test ...` instead of pasting noisy raw test output.
- Avoid generated, vendor, build, dependency lock, and large fixture files unless the task explicitly requires them.
- Do not use git commands.
- Prefer focused tests and minimal, reviewable changes.
