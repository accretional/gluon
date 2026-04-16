# gluon v2

A redesign of gluon's grammar and parsing types. Lives alongside v1
(`github.com/accretional/gluon/pb`, `/lexkit`, `/metaparser`) so we can
migrate callers incrementally; nothing in v2 depends on v1's proto
shapes (the CST implementation reuses v1's *parser engine* internally,
but that is an implementation detail invisible to v2 callers).

Go package root: `github.com/accretional/gluon/v2`
Generated proto types: `github.com/accretional/gluon/v2/pb`
Server + pure-Go entry points: `github.com/accretional/gluon/v2/metaparser`

## What changed vs v1

### 1. Delimiter / Scoper split in the lex model

v1 had a single `LexDescriptor` with a bag of named operator fields. v2
partitions a language's lex into two enums:

- `Delimiter` — positional separators (WHITESPACE, DEFINITION,
  CONCATENATION, TERMINATION, ALTERNATION, …).
- `Scoper` — paired open/close brackets (OPTIONAL, REPETITION, GROUP,
  TERMINAL, COMMENT, …).

`LexicalDelimiter { Delimiter kind; string symbol }` and
`LexicalScoper { Scoper kind; string begin; string end }` carry the
concrete characters. `SymbolDescriptor` is a oneof over those two, and
`LexDescriptor` is just `{ name, repeated SymbolDescriptor symbols }`.

### 2. Flat positional Production encoding

v1 modelled a rule's RHS as a tree: `ProductionExpression` with nested
Sequence / Alternation / Optional / Repetition / Group wrapper
messages. v2 flattens this:

    r = "a" , b | "c"

is encoded as

    RuleDescriptor { name: "r", expressions: [
      Production{ terminal: "a" },
      Production{ delimiter: CONCATENATION },
      Production{ nonterminal: "b" },
      Production{ delimiter: ALTERNATION },
      Production{ terminal: "c" },
    ]}

Delimiters sit *between* their siblings. Scopers wrap a nested
`ScopedProduction { Scoper kind; repeated Production body }`. The wire
form stays closer to the source and removes the
Sequence/Alternation/etc. wrapper layer.

### 3. Chunked text + document model

`DocumentDescriptor { name, uri, repeated TextDescriptor text }` is the
top-level container. `TextDescriptor` is a oneof over four encodings:

- `AsciiChunk` (`repeated ASCII`) — ASCII-only input (compact).
- `UnicodeChunk` (`repeated int32`) — reserved; never emitted by the
  current writers.
- `unicode_string` — UTF-8 string when the input contains non-ASCII.
- `SourceLocation` — by-reference chunk pointing at another document.

`SourceLocation` uses a `uri` (not inline text) deliberately, so the
message can never become a second home for source bytes.

### 4. Literal text encoded via UNSPECIFIED (load-bearing)

`Token`'s oneof over `Delimiter delimiter` / `ScoperToken scoper` has a
canonical "unset" form meaning *literal text run*. Because proto3 cannot
distinguish "oneof unset" from "oneof set to zero", five wire forms all
mean the same thing — producers emit form 1, consumers must accept all
five. This convention is called out with LOAD-BEARING banner comments
in `tokens.proto` and `lex.proto`; reordering the enum zero values or
reusing them as real roles would silently reclassify tokens as text.

### 5. Offset-only tokens; paired scopers via oneof

`Token` carries an `int32 offset` outside the `kind` oneof (always
present) and an optional role. `ScoperToken { oneof kind { Scoper lhs;
Scoper rhs } }` tags open/close sides. Using a oneof rather than a
`{ kind, side }` pair saves a tag+value on the wire per token.

Literal-run length is implied by the next token's offset, so the lexer
must emit a token (typically `Delimiter.WHITESPACE`) for every skipped
run — otherwise literal boundaries are lost.

### 6. `CstRequest` carries a `DocumentDescriptor`

Added during CST implementation. Tokens hold offsets, not text, so
matching a grammar terminal like `"SELECT"` against the source requires
the document itself. The RPC still does not re-lex — tokens remain an
input hint — but the source is the load-bearing input.

## Proto inventory

| File | Messages |
|---|---|
| `source_location.proto` | `SourceLocation { uri, offset, length, line, column }` |
| `text.proto` | `TextDescriptor` (oneof), `AsciiChunk`, `UnicodeChunk` |
| `document.proto` | `DocumentDescriptor { name, uri, repeated TextDescriptor text }` |
| `lex.proto` | `Delimiter`/`Scoper` enums, `LexicalDelimiter`, `LexicalScoper`, `SymbolDescriptor`, `LexDescriptor` |
| `tokens.proto` | `TokenSequence`, `Token`, `ScoperToken` |
| `grammar.proto` | `Production` (oneof), `ScopedProduction`, `StringRange`, `RuleDescriptor`, `GrammarDescriptor` |
| `ast.proto` | `ASTDescriptor`, `ASTNode` |
| `metaparser.proto` | `service Metaparser`, `CstRequest` |

## Metaparser pipeline

    bytes                          → ReadBytes   → TextDescriptor
    string                         → ReadString  → DocumentDescriptor
    DocumentDescriptor (EBNF src)  → EBNF        → GrammarDescriptor
    GrammarDescriptor + Document   → CST         → ASTDescriptor

A separate compiler (not Metaparser) lowers `ASTDescriptor` into a
`FileDescriptorProto` or other target. This is what v1
`metaparser.Build` used to do in one step; in v2 that responsibility
moves out of the metaparser service.

## Current implementation state

All four RPCs are implemented with pure-Go entry points, a unit test
file, and a `_e2e_test.go` that drives the gRPC stack via `bufconn`.

| RPC | Pure-Go entry | Unit tests | E2E tests |
|---|---|---|---|
| `ReadBytes` | `ClassifyBytes(buf) (*TextDescriptor, error)` | 16 | 13 |
| `ReadString` | `WrapString(s) *DocumentDescriptor` | 13 | 12 |
| `EBNF` | `ParseEBNF(doc) (*GrammarDescriptor, error)` | 13 | 9 |
| `CST` | `ParseCST(req) (*ASTDescriptor, error)` | 12 | 10 |

`go test ./v2/metaparser/` is green. The EBNF impl wraps v1's
`lexkit.Parse` and converts v1 `ProductionExpression` trees to v2's
flat `Production` list. The CST impl pretty-prints v2 rules back to
EBNF text, delegates to v1's `lexkit.ParseAST`, and converts the v1
AST to v2's shape. Both are drop-in replacements at the API level;
internally they ride on v1's parser engines.

## Migration plan: replacing v1 in downstream callers

The only external consumer today is **proto-sqlite**. It uses:

- `lexkit.LoadLex` / `lexkit.LoadGrammar` — textproto loaders.
- `lexkit.Parse(src, lex) → GrammarDescriptor` — EBNF parse.
- `lexkit.ToTextproto` — serialize grammar to textproto.
- `metaparser.Build(LanguageDescriptor) → FileDescriptorProto` — the
  EBNF-grammar-to-compilable-proto compiler.

The rough path:

1. **Add a v2 textproto loader.** proto-sqlite keeps
   `sqlite-lex.textproto` / `sqlite-grammar.textproto`; we need a
   v2-equivalent that parses into the v2 messages. Either translate
   the existing textprotos to v2 shape, or add a `loadV2Textproto`
   helper in `v2/metaparser` that unmarshals directly.

2. **Switch proto-sqlite's grammar pipeline to v2.** In
   `lang/cmd/gengrammar/main.go` and `lang/cmd/genproto/main.go`,
   replace the v1 `lexkit.Parse` call with a v2 client call to
   `ReadString` + `EBNF`. The output textproto moves from v1
   `GrammarDescriptor` to v2 `GrammarDescriptor`.

3. **Replace `metaparser.Build` with a v2 compiler.** This is the big
   missing piece: v2 intentionally keeps the metaparser to lex/parse
   only, and puts the "AST → FileDescriptorProto" step in a separate
   compiler (see the flow comment in `metaparser.proto`). The v1
   `metaparser/metaparser.go` is ~400 lines of lowering logic that
   walks `ProductionExpression` trees and emits descriptor messages;
   we need a v2 equivalent that walks `ASTDescriptor` (or perhaps
   `GrammarDescriptor` directly — TBD) and emits the same output.

4. **Port v1 lexkit helpers callers still need.** Only
   `LoadLex` / `LoadGrammar` / `ToTextproto` are used externally; the
   raw UTF8 helpers (`Char`, `RuneOf`, `RawToToken`, `TokenToRaw`)
   are v1-internal. Loaders need v2 equivalents; the UTF8 helpers can
   stay in v1 for now since v2 proto types don't use them.

5. **Delete the v1 package once callers are off it.** v1 `/pb`,
   `/lexkit`, `/metaparser` can be removed once proto-sqlite no longer
   imports them. Until step 3 lands, v2's CST implementation will
   continue to depend on v1 internally (as a parser engine), so v1
   cannot be fully deleted yet — but it can be marked internal.

## Open work

- **v2 compiler** — grammar/AST → `FileDescriptorProto`. The next
  real feature; blocks step 3 of the migration plan.
- **Native v2 parser** — replace the v1-parser shim inside
  `ParseCST`. Would let us delete v1 wholesale. Not urgent; the shim
  is ~150 lines and works.
- **`TokenSequence` integration** — currently `CstRequest.tokens` is
  accepted but unused. Could drive pre-lex skipping of whitespace /
  comments once a native v2 parser exists.
- **Wire v2 into proto-sqlite** — tracked as task #13 in this repo's
  task list.
