package cli

const Usage = `goctx - Go repository context optimizer for Codex

Commands:
  help                              Print this help.
  scan                              Scan repository and classify files.
  discover                          Discover Go modules/workspaces and selected module.
  map                               Print compact Go package map.
  symbols <file>                    Print symbols from one Go file.
  slice <file> --symbol <name>      Print one symbol body from a Go file.
  task "<task description>"         Print a heuristic context pack.
  codex-pack "<task description>"   Print a budgeted one-command Codex context pack.
  routes                            Extract likely HTTP route registrations.
  test-summary -- <go command>      Run and summarize a Go test command.
  diff-summary                      Summarize changes from .goctx/baseline.json if present.
  baseline create|update|status     Manage filesystem baseline snapshots.
  token-report                      Estimate token risk by file.
  doctor                            Check goctx readiness in the current repository.
  smells                            Report token/navigation risk smells.
  init-codex [--force] [--strict]   Generate conservative Codex integration files.

Common flags on context commands:
  --root <path> --module <path-or-name> --all-modules
  --budget <estimated-tokens> --max-files <n> --max-symbols <n> --max-routes <n>
  --format text|json|markdown
`
