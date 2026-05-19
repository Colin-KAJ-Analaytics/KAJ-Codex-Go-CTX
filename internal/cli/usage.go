package cli

const Usage = `goctx - Go repository context optimizer for Codex

Commands:
  help                              Print this help.
  scan                              Scan repository and classify files.
  map                               Print compact Go package map.
  symbols <file>                    Print symbols from one Go file.
  slice <file> --symbol <name>      Print one symbol body from a Go file.
  task "<task description>"         Print a heuristic context pack.
  routes                            Extract likely HTTP route registrations.
  test-summary -- <go command>      Run and summarize a Go test command.
  diff-summary                      Summarize changes from .goctx/baseline.json if present.
  token-report                      Estimate token risk by file.
  init-codex [--force]              Generate conservative Codex integration files.
`
