package metaparser

// Reusable CST-parsing benchmarks (see PERF.md for the methodology and
// scripts/bench-parse.sh for the driver).
//
// Two modes:
//
//  1. Synthetic (always on): a built-in key:value;key:value;... grammar and
//     generated inputs of 100 / 1,000 / 10,000 statements. The sub-benchmark
//     RATIOS are the signal: 10x more input should cost ~10x more time; a
//     ratio far above that means parsing has gone superlinear (this suite
//     reproduces lexkit's O(n^2) loc() — issue #6). The input is a single
//     line with no whitespace: every rule is lowercase (= lexical mode, no
//     implicit skipping), which keeps the grammar fully explicit — and the
//     old loc() rescan was O(pos) per node with or without newlines.
//
//  2. External (opt-in): point GLUON_BENCH_GRAMMAR at any .ebnf and
//     GLUON_BENCH_INPUTS at a file glob to bench YOUR grammar against YOUR
//     corpus with plain ParseCST. Grammars whose lexical atoms need token
//     matchers (ParseCSTWithOptions) can't be driven from env vars — vendor
//     this harness next to your matcher table instead; see
//     github.com/accretional/proto-robotstxt/bench for a worked example.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/accretional/gluon/v2/pb"
)

// benchGrammar is a minimal statement-oriented grammar. All-lowercase rule
// names put every production in lexical mode: nothing is skipped implicitly,
// so the grammar (and the whitespace-free input) is fully explicit.
const benchGrammar = `
doc   = { stmt } ;
stmt  = key , ":" , value , ";" ;
key   = kchar , { kchar } ;
value = vchar , { vchar } ;
kchar = "a" ... "z" | "-" ;
vchar = "a" ... "z" | "0" ... "9" | "." | "-" ;
`

func synthInput(stmts int) string {
	var b strings.Builder
	for i := 0; i < stmts; i++ {
		fmt.Fprintf(&b, "key-%s:value-%s.%d;", alpha(i), alpha(i*7), i%10)
	}
	return b.String()
}

// alpha renders n in base-26 letters (grammar keys allow only a-z and '-').
func alpha(n int) string {
	if n == 0 {
		return "a"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('a' + n%26)}, out...)
		n /= 26
	}
	return string(out)
}

func benchParse(b *testing.B, gd *pb.GrammarDescriptor, name, src string) {
	b.Run(name, func(b *testing.B) {
		b.SetBytes(int64(len(src)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			doc := WrapString(src)
			if _, err := ParseCST(&pb.CstRequest{Grammar: gd, Document: doc}); err != nil {
				b.Fatalf("ParseCST: %v", err)
			}
		}
	})
}

func BenchmarkParseCSTSynthetic(b *testing.B) {
	doc := WrapString(benchGrammar)
	doc.Name = "bench.ebnf"
	gd, err := ParseEBNF(doc)
	if err != nil {
		b.Fatalf("ParseEBNF: %v", err)
	}
	for _, stmts := range []int{100, 1000, 10000} {
		benchParse(b, gd, fmt.Sprintf("stmts=%d", stmts), synthInput(stmts))
	}
}

// BenchmarkParseCSTExternal benches an arbitrary grammar + corpus:
//
//	GLUON_BENCH_GRAMMAR=path/to/grammar.ebnf \
//	GLUON_BENCH_INPUTS='path/to/corpus/*.txt' \
//	go test -run '^$' -bench ParseCSTExternal ./v2/metaparser/
func BenchmarkParseCSTExternal(b *testing.B) {
	grammarPath := os.Getenv("GLUON_BENCH_GRAMMAR")
	inputsGlob := os.Getenv("GLUON_BENCH_INPUTS")
	if grammarPath == "" || inputsGlob == "" {
		b.Skip("set GLUON_BENCH_GRAMMAR and GLUON_BENCH_INPUTS to bench an external grammar")
	}
	src, err := os.ReadFile(grammarPath)
	if err != nil {
		b.Fatal(err)
	}
	doc := WrapString(string(src))
	doc.Name = grammarPath
	gd, err := ParseEBNF(doc)
	if err != nil {
		b.Fatalf("ParseEBNF %s: %v", grammarPath, err)
	}
	files, err := filepath.Glob(inputsGlob)
	if err != nil || len(files) == 0 {
		b.Fatalf("GLUON_BENCH_INPUTS %q matched no files (err=%v)", inputsGlob, err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			b.Fatal(err)
		}
		benchParse(b, gd, filepath.Base(f), string(data))
	}
}
