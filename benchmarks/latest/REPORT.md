# goctx backtest — November 7, 2026

## Verdict

The agreed first-phase changes pass the behavioral specification exercised here. Compact mode materially reduces emitted context. There is no evidence of a general speed-up over the installed baseline. Default verbose output is not consistently smaller. A JSON sizing optimization made during this backtest removes avoidable work while preserving the earlier updated build’s output exactly.

## Validation

- Full Go test suite passed with `go test -count=1 ./...`; `go vet ./...` and the build passed.
- 25/25 executable-level behavioral checks passed across diagnostics, budgets, route ranking, usable read plans, compact output, deterministic output, and Claude setup.
- 16/16 matched workloads produced byte-identical default output before and after the incremental JSON sizing optimization.
- 210 serialization comparisons passed in the new Go test: pretty/compact output, escaped and Unicode strings, empty/nonempty arrays, prior omissions, and omission count digit boundaries.
- Small-budget checks permit metadata-only overflow only when it is explicitly warned and no optional context items are returned. Otherwise emitted size must fit the budget.

## Measured context output

All token figures below are `UTF-8 bytes // 4`, not model tokenizer counts. The reduction is measured in emitted bytes; the estimated-token percentages are effectively the same. These are output-size comparisons, not total agent-session cost or billing savings.

| Repository | Command | Installed baseline estimate | New default estimate | New compact estimate | Compact reduction vs baseline |
|---|---|---:|---:|---:|---:|
| real_repo | task_text | 781 | 645 | 403 | 48.3% |
| real_repo | pack_text | 590 | 645 | 403 | 31.6% |
| real_repo | pack_json | 947 | 1,104 | 569 | 39.9% |
| small | task_text | 783 | 693 | 380 | 51.4% |
| small | pack_text | 642 | 700 | 387 | 39.8% |
| small | pack_json | 1,149 | 1,439 | 608 | 47.1% |
| medium | task_text | 1,828 | 1,630 | 383 | 79.0% |
| medium | pack_text | 1,625 | 1,637 | 390 | 76.0% |
| medium | pack_json | 2,318 | 1,996 | 627 | 72.9% |
| large | task_text | 2,000 | 1,815 | 384 | 80.8% |
| large | pack_text | 1,797 | 1,822 | 391 | 78.2% |
| large | pack_json | 2,488 | 1,997 | 628 | 74.7% |

A synthetic noisy failure shrank from **21,807 to 2,154 bytes (90.1%)**, while preserving failure status and reporting omitted diagnostics. This is a deliberately noisy stress case, not an average failure.

Compact text means `--quiet`. Compact JSON means `--quiet --compact-json --format json`. These flags are opt-in. At a generous budget, pretty and compact JSON were decoded and compared to verify identical semantic content, excluding their size field.

## Runtime

Nine warm-cache trials per variant were run against the installed baseline, with paired variant order shuffled using a fixed seed. The table gives wall-clock medians in milliseconds. There were no concurrent build/test jobs during timing. This is a local descriptive benchmark, not a statistical proof of small timing differences.

| Repository | Command | Installed baseline ms | New default ms | Change |
|---|---|---:|---:|---:|
| real_repo | task_text | 81.52 | 80.95 | -0.7% |
| real_repo | pack_text | 91.75 | 91.77 | +0.0% |
| real_repo | pack_json | 92.30 | 91.86 | -0.5% |
| real_repo | map_json | 79.95 | 78.77 | -1.5% |
| small | task_text | 61.27 | 61.53 | +0.4% |
| small | pack_text | 62.17 | 61.83 | -0.6% |
| small | pack_json | 62.38 | 62.08 | -0.5% |
| small | map_json | 59.17 | 61.19 | +3.4% |
| medium | task_text | 68.67 | 70.11 | +2.1% |
| medium | pack_text | 72.86 | 74.02 | +1.6% |
| medium | pack_json | 73.14 | 74.46 | +1.8% |
| medium | map_json | 67.77 | 68.16 | +0.6% |
| large | task_text | 106.61 | 107.10 | +0.5% |
| large | pack_text | 121.66 | 120.93 | -0.6% |
| large | pack_json | 121.51 | 121.35 | -0.1% |
| large | map_json | 99.08 | 99.37 | +0.3% |

The final candidate ranged from approximately 1.5% faster to 3.4% slower than the installed baseline across these workloads. Treat that as broadly comparable runtime. Some corrected responses carry more useful information. For example, the real-repository JSON map returned 20 files instead of one, with 40 symbols in both versions; it is larger, but that is not an equivalent-content regression.

## Performance correction made during validation

The first implementation repeatedly serialized the entire growing JSON response while considering each item. The final implementation encodes each candidate item once, accounts for array formatting and omission counts incrementally, then verifies the final serialized response. It retains actual-byte-based estimates and explicit metadata-overflow warnings.

A separate seven-trial paired comparison used the exact executable from before this correction as its baseline. All 16 default workload outputs were byte-identical. JSON runtime results were:

| Repository | Command | Before sizing fix ms | After sizing fix ms | Median improvement |
|---|---|---:|---:|---:|
| real_repo | pack_json | 92.57 | 91.44 | 1.2% |
| real_repo | map_json | 85.34 | 80.64 | 5.5% |
| small | pack_json | 65.44 | 61.80 | 5.6% |
| small | map_json | 63.94 | 60.06 | 6.1% |
| medium | pack_json | 77.07 | 72.48 | 6.0% |
| medium | map_json | 71.57 | 67.98 | 5.0% |
| large | pack_json | 127.41 | 121.70 | 4.5% |
| large | map_json | 104.27 | 99.25 | 4.8% |

## Specification coverage

| Item | Evidence |
|---|---|
| D1 full subtest names | Executed nested failing subtest with punctuation; full path retained. |
| D2 trustworthy status | Executed a passing test containing FAIL in its name/log and a compile failure; checked exit status and actual diagnostic. |
| D3 bounded diagnostics | Executed a failure emitting 1,200 reference lines; summary stayed below 12 KB with omissions. |
| D4 serialized accounting | Checked exact byte-derived estimates across 250/500/1,000/2,000 budgets and output modes, plus explicit impossible-budget behavior. |
| D5 relevant routes | Selected `/login` ahead of the unrelated first `/health` route under a one-route limit. |
| D6 symbol locations | Preserved four same-named symbols in different files; executed emitted slice commands successfully. |
| D7 useful text truncation | A 250-token text pack retained read actions and focused tests. |
| G6 / E5 compact output | Static advice/reasons removed, warning/omission fields retained; decoded JSON equivalence checked. |
| G9 import fallback | Full suite includes Go-syntax imports with aliases, blank/dot imports, raw literals, and misleading non-import strings. |
| E10 stable output | Repeated packs were byte-stable, including during every timed workload. |
| Claude integration | CLI preservation/idempotence exercised; Go tests cover fresh setup, strict policy, `.claude` entry points, import examples, and symlink refusal. |

## Scope and limits

- Baseline: existing October 22 installed executable, not a reconstructed snapshot immediately before the reviewed changes. Its toolchain and dependency versions match the candidate (Go 1.26.2, darwin/arm64). The before/after attribution for the original changes is therefore approximate.
- Workloads: this real repository plus controlled fixtures of 13, 49, and 209 Go files. The fixtures have predictable relevant symbols/routes; they are not three independent public production repositories.
- Runtime: warm-cache CLI wall time. Cold filesystem caches, memory/CPU profiles, other platforms, and independent agent task-completion costs were not measured.
- Tokenization: no model tokenizer was available. The benchmark measures UTF-8 output bytes and the tool’s documented byte-based estimate.
- Claude: instruction-file behavior was tested, but no live Claude session was launched to confirm client-side loading.
- References/callers, package API queries, caching, and the other deferred roadmap items are outside this implemented phase.
- `/usr/local/bin/goctx` remains the old installed version; the validated new executable is `bin/goctx`.

## Reproduce

```sh
GOCACHE=/tmp/goctx-review-cache GOPROXY=off GOFLAGS=-buildvcs=false go build -o bin/goctx ./cmd/goctx
python3 scripts/backtest.py --baseline /usr/local/bin/goctx --candidate ./bin/goctx --repeats 9
```

The harness saves raw results, binary hashes, individual timing samples, and a CSV. It creates disposable local fixtures and does not run git commands or download dependencies. Preserve the old executable separately before replacing the installation if future comparisons are needed.

Use the optimized modes from this repository:

```sh
./bin/goctx task "your task" --quiet --budget 2000
./bin/goctx codex-pack "your task" --quiet --format json --compact-json --budget 2000
```
