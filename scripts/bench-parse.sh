#!/usr/bin/env bash
# bench-parse.sh — reusable driver for gluon's CST-parsing benchmarks
# (v2/metaparser/parse_bench_test.go). See PERF.md for how to read results.
#
# Usage:
#   scripts/bench-parse.sh                       # synthetic scaling suite
#   scripts/bench-parse.sh -p                    # + CPU profile, print pprof top
#   scripts/bench-parse.sh -g g.ebnf -i 'x/*.txt'  # bench an external grammar/corpus
#   scripts/bench-parse.sh -t 5x                 # override -benchtime
#
# The synthetic suite's scaling RATIOS are the health signal: stmts=1000
# should cost ~10x stmts=100 (linear). A 30x+ ratio means parsing has gone
# superlinear — profile with -p and look at the top frame.
#
# Grammars whose lexical atoms need registered token matchers
# (ParseCSTWithOptions) cannot be benched via -g; vendor the harness next to
# your matcher table instead (worked example:
# github.com/accretional/proto-robotstxt/bench).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GRAMMAR="" INPUTS="" PROFILE=0 BENCHTIME="" BENCH_RE='BenchmarkParseCST'

while getopts "g:i:pt:b:h" opt; do
  case "$opt" in
    g) GRAMMAR="$OPTARG" ;;
    i) INPUTS="$OPTARG" ;;
    p) PROFILE=1 ;;
    t) BENCHTIME="$OPTARG" ;;
    b) BENCH_RE="$OPTARG" ;;
    h|*) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  esac
done

if [ -n "$GRAMMAR" ]; then
  [ -n "$INPUTS" ] || { echo "bench-parse: -g requires -i (input glob)" >&2; exit 2; }
  # Absolute path: go test runs in the package dir, not $PWD.
  export GLUON_BENCH_GRAMMAR="$(cd "$(dirname "$GRAMMAR")" && pwd)/$(basename "$GRAMMAR")"
  export GLUON_BENCH_INPUTS="$INPUTS"
  [ "$BENCH_RE" = 'BenchmarkParseCST' ] && BENCH_RE='BenchmarkParseCSTExternal'
fi

args=(test -run '^$' -bench "$BENCH_RE" -benchmem)
[ -n "$BENCHTIME" ] && args+=(-benchtime "$BENCHTIME")

out_dir="$(mktemp -d "${TMPDIR:-/tmp}/gluon-bench.XXXXXX")"
prof="$out_dir/cpu.prof"
bin="$out_dir/metaparser.test"
if [ "$PROFILE" -eq 1 ]; then
  args+=(-cpuprofile "$prof" -o "$bin")
fi
args+=(./v2/metaparser/)

echo "[bench-parse] go ${args[*]}"
(cd "$REPO_ROOT" && go "${args[@]}")

if [ "$PROFILE" -eq 1 ]; then
  echo
  echo "[bench-parse] CPU profile top frames ($prof):"
  (cd "$REPO_ROOT" && go tool pprof -top -nodecount=15 "$bin" "$prof" 2>/dev/null | sed -n '5,25p')
  echo "[bench-parse] explore interactively: go tool pprof $bin $prof"
fi
