# PERF.md — benchmarking & profiling gluon's grammar-driven parser

How to measure CST-parsing performance, how to read the numbers, and the
worked example that produced this file (issue #6 — the O(n²) `loc()`
rescan, fixed on main in `3b97bbf`).

## Quick start

```sh
scripts/bench-parse.sh          # synthetic scaling suite (always available)
scripts/bench-parse.sh -p       # same + CPU profile, prints pprof top frames
```

The suite lives in `v2/metaparser/parse_bench_test.go` and parses a built-in
statement-oriented grammar at 100 / 1,000 / 10,000 input statements.

## Reading the results: scaling ratios, not absolute numbers

Absolute ns/op depends on the machine; the **ratio between size steps** does
not. Each step multiplies input size by 10, so:

| ratio per 10× input | meaning |
|---|---|
| ~10× | linear — healthy |
| ~30×+ | superlinear — something rescans or re-parses |
| ~100× | quadratic — a per-node O(input) walk; profile it |

`-benchmem` allocation counts scaling linearly while time scales
quadratically is the classic signature of a CPU-side rescan (allocations
follow node count; time follows node count × scan length).

## Profiling a regression

```sh
scripts/bench-parse.sh -p
# or by hand:
go test -run '^$' -bench 'ParseCSTSynthetic/stmts=1000$' -benchtime=2x \
  -cpuprofile cpu.prof -o metaparser.test ./v2/metaparser/
go tool pprof -top -nodecount=15 metaparser.test cpu.prof
```

Use a size where the slowdown is pronounced but the run stays short
(`stmts=1000`, `-benchtime=2x` — profiles need ~1s of samples, not minutes).
One dominant flat frame = your culprit; `-list <func>` shows the hot lines:

```sh
go tool pprof -list 'astParser.loc' metaparser.test cpu.prof
```

## Benching your own grammar

```sh
scripts/bench-parse.sh -g path/to/grammar.ebnf -i 'path/to/corpus/*.txt'
```

This drives plain `ParseCST`. Grammars whose lexical atoms are registered
token matchers (`ParseCSTWithOptions`) cannot be loaded from a path — vendor
the harness next to your matcher table; a worked example (RFC 9309
robots.txt, matcher-heavy) is `bench/` in
[accretional/proto-robotstxt](https://github.com/accretional/proto-robotstxt).

## Worked example: issue #6 (the reason this file exists)

Downstream scaling benchmarks on a robots.txt grammar showed 6.5 ms/100
lines → 425 ms/1k (65×) → 43.7 s/10k (103×): quadratic. A 1.3 s profile of
the 1k case put **`(*astParser).loc` at 61.7% flat CPU**: it recomputed
line/column by rescanning the source from offset 0 for *every emitted node*
— O(pos) per call, O(n²) per parse.

The fix (`3b97bbf`, `lexkit/parse_ast.go`): `newASTParser` precomputes
`lineStarts` once; `loc()` binary-searches it. Behavior-identical — columns
still count bytes; `lexkit/parse_ast_loc_test.go` proves equivalence against
the old linear loop for every position over edge-case sources. Synthetic
suite on an Apple M4 (`-benchtime=3x`), before → after:

| stmts | before | after | speedup |
|---|---|---|---|
| 100 | 2.27 ms | 0.71 ms | 3.2× |
| 1,000 | 173 ms | 8.2 ms | 21× |
| 10,000 | 19.24 s | 83.6 ms | **230×** |

Downstream (proto-robotstxt's matcher-driven RFC 9309 grammar): 10k-line
parse 43.7 s → 105 ms (~415×). After the fix the ratios are ~10×/10×
(linear) at constant MB/s, and the allocation counts are byte-identical to
before — only CPU changed. Rerun `scripts/bench-parse.sh` to reproduce on
your machine.
