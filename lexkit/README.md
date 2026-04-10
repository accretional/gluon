# lexkit

lexkit is a bootstrapped EBNF grammar toolkit. It parses EBNF grammar
definitions into Protocol Buffer descriptors (`GrammarDescriptor`), using a
`LexDescriptor` that configures how the EBNF notation itself is read.

## Status: bootstrapped

All three LexDescriptors (EBNF, Go, Proto) are serialized as binary protobuf
(`.binarypb`) and loaded via `//go:embed`. There is no hardcoded lexer
configuration in Go — `lexkit.go` is pure parser + proto helpers.

The EBNF grammar is self-hosting: `ebnf.binarypb` was used to parse
`ebnf.txt` (the EBNF grammar of EBNF), producing `ebnf_grammar.textproto` —
which can re-parse `ebnf.txt` and produce identical results.

## How it works

```
ebnf.txt (EBNF-of-EBNF source)
    |
    v
Parse(src, EBNFLex()) --> *pb.GrammarDescriptor
    |                          |
    v                          v
ebnf_grammar.textproto    ebnf.binarypb (LexDescriptor only)
                               |
                               v
                          //go:embed --> EBNFLex()
```

1. `EBNFLex()` loads the EBNF `LexDescriptor` from `ebnf.binarypb`.
2. `Parse(src, lex)` reads EBNF source and returns a `*pb.GrammarDescriptor`
   containing the `LexDescriptor` and all `ProductionDescriptor`s.
3. `ToTextproto(gd)` serializes a `GrammarDescriptor` via `prototext.Marshal`.
4. `LoadGrammar(path)` reads a textproto back into a `GrammarDescriptor`.

## Files

| File | Description |
|------|-------------|
| `lexkit.go` | Parser, LexDescriptor loaders, Char/RuneOf/Token helpers |
| `ebnf.binarypb` | Embedded binary protobuf of the EBNF LexDescriptor |
| `go.binarypb` | Embedded binary protobuf of the Go LexDescriptor |
| `proto.binarypb` | Embedded binary protobuf of the Proto LexDescriptor |
| `ebnf.txt` | ISO 14977 EBNF grammar written in its own notation |
| `ebnf_grammar.textproto` | Generated: full GrammarDescriptor for EBNF |
| `go_ebnf.txt` | Go language EBNF grammar |
| `go_grammar.textproto` | Generated: full GrammarDescriptor for Go |
| `proto_ebnf.txt` | Protocol Buffers EBNF grammar |
| `proto_grammar.textproto` | Generated: full GrammarDescriptor for Proto |
| `cmd/lexgen/` | CLI that parses all grammars and writes textproto files |

## Dev loop

```sh
# Parse all grammars and regenerate textproto files:
go run ./lexkit/cmd/lexgen ./lexkit

# Run tests (includes self-hosting validation):
go test ./lexkit/...
```

The `lexgen` command parses each EBNF source file with its corresponding
`LexDescriptor`, writes the `GrammarDescriptor` as textproto, runs any
available validators (x/exp/ebnf cross-check for Go, self-describing check
for EBNF), and finally validates self-hosting by loading
`ebnf_grammar.textproto` and using its `LexDescriptor` to re-parse `ebnf.txt`.

## Proto types

All types live in `grammar.proto` and `lex.proto`, with character encoding
via `unicode/ascii.proto` and `unicode/utf_8.proto` (vendored from
[proto-symbol](https://github.com/accretional/proto-symbol)):

- **`GrammarDescriptor`** — a `LexDescriptor` + repeated `ProductionDescriptor`
- **`LexDescriptor`** — configures EBNF delimiters (definition, termination,
  alternation, brackets, quotes, comments) as `UTF8` values
- **`ProductionDescriptor`** — production name + `TokenDescriptor` body
- **`TokenDescriptor`** — repeated `UTF8` chars encoding the EBNF expression
- **`UTF8`** — oneof `{ASCII ascii, uint32 symbol}` — ASCII range uses
  readable enum names (e.g. `VERTICAL_LINE`, `EQUALS_SIGN`)
