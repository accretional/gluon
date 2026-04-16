# Plan: v2 proto-lowering compiler (`Metaparser.Compile`)

Status: **proposal, not yet started.** Written for review.

## Context

gluon v2 currently exposes `ReadBytes`, `ReadString`, `EBNF`, `CST`,
and `Transform`. Together these go from source text → `ASTDescriptor`
and let scripts rewrite the AST. What's missing is the **lowering
step**: `ASTDescriptor → FileDescriptorProto`. v1's
`metaparser.Build` does this for gluon v1 grammars; v2 has no
equivalent, which is why `proto-sqlite/lang/cmd/genproto` still
imports v1 `lexkit.Parse` + `metaparser.Build`.

This plan specifies a v2 proto-lowering compiler that:

1. Takes an `ASTDescriptor` (not a `GrammarDescriptor`) so the same
   lowering works for any source language, not just EBNF.
2. Ships as a pure-Go package, a standalone `Metaparser.Compile`
   RPC, and a `protoc://Compile` handler inside `Transform` — three
   surfaces over the same implementation.
3. Cuts proto-sqlite over with one commit cycle of parallel v1/v2
   running, then v1 goes away.

## Why ASTDescriptor, not GrammarDescriptor

Taking `GrammarDescriptor` would be simpler today — the grammar is
already structured and typed. But it would specialize the compiler
to "gluon grammars only", which defeats the v2 project's stated
goal: **cross-language codegen/transformations done on a universal
AST**. Concrete future-use cases that a GrammarDescriptor input
rules out:

- Compile a Go struct AST → proto messages (what v1
  `struct2proto/` does in v1's codepath; in v2 it should be one
  more `Compile` call over a different AST).
- Compile a captnproto/thrift/OpenAPI schema AST → proto messages.
- Let a Transform pipeline run AST rewrites (via `astkit://`)
  *before* lowering, without having to round-trip back to a typed
  grammar shape.

Cost of taking AST instead of grammar: the compiler has to interpret
the AST by convention — specifically, it expects the AST's `kind`
labels to match the conventions below. For EBNF-sourced ASTs, the
conventions are whatever `Metaparser.CST` emits when parsing an
EBNF document; a tiny `GrammarDescriptor → ASTDescriptor` helper
makes the grammar path a one-liner for callers who still start
with a `GrammarDescriptor`.

## AST kind conventions

The compiler recognizes these node `kind` strings. Any other kind
at an expected position is an error. Exact names TBD during
implementation; this is the rough shape.

| Role | `kind` | Structure |
|---|---|---|
| File root | `file` | children = list of rules |
| Production / rule | `rule` | `value` = rule name; children = one expression |
| Sequence (concat) | `sequence` | children = items in order |
| Alternation | `alternation` | children = variants |
| Optional | `optional` | one child |
| Repetition | `repetition` | one child |
| Group | `group` | one child (transparent wrapper) |
| Terminal (quoted literal) | `terminal` | `value` = literal string |
| Nonterminal (identifier) | `nonterminal` | `value` = rule name |
| Range | `range` | `value` = "low..high" or two children |

The conventions mirror the EBNF structure 1:1. Non-EBNF source
languages can either (a) produce ASTs that use these kinds, or (b)
be normalized with a Transform pass first (`astkit://ReplaceKind`
and friends are already wired for this).

## Mapping to FileDescriptorProto

Same rules as v1 `metaparser.Build`, adapted to the AST shape:

| AST | proto |
|---|---|
| `rule` P | `message PascalCase(P)` |
| `sequence` children | fields 1..N in order |
| `alternation` children | single `oneof` with one field per variant |
| `optional` child | singular field (proto3 implicit optional) |
| `repetition` child | `LABEL_REPEATED` on the field |
| `group` child | inlined transparently |
| `terminal` "FOO" | dedup'd empty `message FooKw {}`, field name `foo_kw` |
| `nonterminal` "foo" | field `foo` typed `.pkg.Foo` |
| `range` | two `unicode.UTF8` bounds fields; marks file dep on `unicode/utf_8.proto` |

Name-collision rules from SUPERPLAN apply: `_Kw` suffix on all
keyword messages, snake_case field names, recursive productions
allowed directly.

## Design surface

### Pure-Go package: `gluon/v2/compiler/`

```go
package compiler

type Options struct {
    Package   string // proto package; default: ast.GetLanguage() or "lang"
    GoPackage string // go_package option; default: "" (omit)
    FileName  string // file descriptor name; default: Package + ".proto"
}

func Compile(ast *pb.ASTDescriptor, opts Options) (*descriptorpb.FileDescriptorProto, error)
```

One exported function, deterministic, no I/O. Internally: walk the
AST root, emit one `DescriptorProto` per `rule` node, dedup
keywords, assemble file descriptor, return.

### RPC: `Metaparser.Compile`

Added to `v2/metaparser.proto`:

```proto
rpc Compile(CompileRequest) returns (CompileResponse);

message CompileRequest {
  ASTDescriptor ast = 1;
  string package = 2;
  string go_package = 3;
  string file_name = 4;
}

message CompileResponse {
  google.protobuf.FileDescriptorProto file = 1;
}
```

Server wraps the pure-Go `Compile`. Error translation:
`InvalidArgument` for unknown kinds / malformed AST, `Internal` for
bugs in the walker.

### Transform handler: `protoc://Compile`

Registered inside `metaparser/transform.go` alongside
`astkit://*`. Convention:

- `Data.binary` = marshaled `ASTDescriptor` (in whatever register
  the script passed as the `request.text`).
- `Data.type` carries `package=X,go_package=Y,file_name=Z` using
  the same `k=v,k2=v2` convention already used by `astkit://*`.
- Response: `Data{type: "google.protobuf.FileDescriptorProto", binary: <marshaled fdp>}`.

Lets a script chain lowering with prior AST rewrites without
leaving `Transform`:

```textproto
statements: { dispatch: { uri: "astkit://Filter"         ... name: "ast" } }
statements: { dispatch: { uri: "astkit://ReplaceKind"    ... name: "ast" } }
statements: { dispatch: { uri: "protoc://Compile"
  request: { type: "package=sqlite,go_package=github.com/accretional/proto-sqlite/protos", text: "ast" }
  path: "lang/sqlite.fdset"
}}
```

### Other handlers worth wiring (deferred)

Out of scope for this work. Noted so the pattern is documented
for the next person to need them:

- **`protomerge://Transform`** — post-compile renames / field
  surgery via `proto-merge`'s `Merger.Transform`. Wire when a
  real caller needs it.
- **Go codegen handlers** (`go://Format` etc.) — explicitly out
  of scope. This project is proto-only: AST →
  `FileDescriptorProto`, stop there. `protoc` itself handles
  `.proto` → `.go`.

## Tests

### Pure-Go (`compiler/compiler_test.go`)

- One case per AST kind: tiny handcrafted `ASTDescriptor`, assert
  the emitted `DescriptorProto` has the expected fields / labels /
  oneofs.
- Keyword dedup: two rules referring to `"SELECT"` as a literal,
  assert one shared `SelectKw` message.
- Recursive rules: `expr = term { ("+" | "-") term }` — assert
  `Expr` message with a self-reference, no infinite loop in the
  walker.
- Range: `digit = "0".."9"` — assert two `UTF8` fields, file dep
  on `unicode/utf_8.proto`.
- Options plumbing: pass `Package="x",GoPackage="y/z"`, assert
  they land on the `FileDescriptorProto`.
- Self-hosting: walk an EBNF grammar's CST through `Compile`,
  assert the output has one message per production.

### RPC (`metaparser/compile_e2e_test.go`, bufconn)

- Happy path with a small AST, assert the returned
  `FileDescriptorProto` matches a checked-in fixture.
- Malformed AST (unknown `kind`) returns `InvalidArgument`.

### Transform handler
(`metaparser/transform_test.go` additions)

- Register `protoc://Compile`, call with a register-backed AST and
  `Data.type` params, assert the response decodes to a
  `FileDescriptorProto`.
- Chained: `astkit://ReplaceKind` → `protoc://Compile`, assert the
  renamed kinds show up as renamed messages.

## Cutover in proto-sqlite

Goal: delete v1 from proto-sqlite without a flag day. Two commits.

**Commit 1 — parallel:**
1. Add `lang/cmd/genproto-v2/main.go`. Calls v2 `ReadString` →
   `EBNF` → (convert `GrammarDescriptor` to `ASTDescriptor` via a
   new helper) → `Compile` → write `.fdset` + split via existing
   `proto-merge/descriptor` splitter.
2. Extend `build.sh` to run both `genproto` (v1) and `genproto-v2`,
   writing to separate paths (`lang/sqlite.fdset` vs
   `lang/sqlite.v2.fdset`). Add a `diff -q` between them. CI fails
   only if they disagree beyond known cosmetic diffs.
3. Commit gluon + proto-sqlite together, ship.

Cosmetic diffs expected: field comments, whitespace in the
textproto serialization. Behavioral diffs (different field
numbers, different message types) are bugs and must be resolved
before commit 1 lands.

**Commit 2 — cutover:**
1. Delete `lang/cmd/genproto/`. Rename `genproto-v2/` → `genproto/`.
2. Update `build.sh` to stop writing `lang/sqlite.v2.fdset` —
   `lang/sqlite.fdset` is now the v2 output.
3. Delete `gluon/metaparser/` once no callers remain (proto-sqlite
   is the only known caller; confirm with `grep` across sibling
   repos). If `metaparser.Build` is still imported elsewhere,
   mark v1 internal instead of deleting and delete next cycle.
4. Remove the `gluon/metaparser` entry from gluon's root README
   package table.

## Phase ordering

1. Phase A (`compiler/`) + Phase B (`Compile` RPC) + Phase C
   (`protoc://Compile` handler) in gluon, one PR. `go test ./v2/...`
   green.
2. Push gluon, bump proto-sqlite's gluon dep.
3. Cutover commit 1 in proto-sqlite. `build.sh` green with diff
   check.
4. Cutover commit 2 in proto-sqlite + gluon. v1 deleted.

## Open questions

- **Package-name defaulting.** v1 uses `ld.GetName()`. v2 AST root
  has a `language` field; reusing that seems right, but it needs
  to be populated when the AST comes from non-EBNF sources. OK as
  a default with explicit override.
- **Field-number stability.** v1 assigns field numbers in source
  order. Same behavior in v2 means any edit to `sqlite.ebnf` can
  renumber fields. Acceptable for now; a `.fieldmap` textproto for
  stable assignments is a separate future task.
- ~~**Does `Metaparser.CST` currently emit the AST kinds listed
  above when parsing EBNF?**~~ Checked: no. v2 `CST` produces a
  *parse tree* where node kinds are matched rule names (e.g.
  `greet`, `hello`, `world` for a grammar `greet = hello , world`).
  That's correct for a CST — it represents the parse of source
  *against* a grammar, not the grammar itself. The compiler needs
  a different kind of AST: one whose nodes describe schema
  constructs (`rule`, `sequence`, `alternation`, …). So we add a
  `grammar2ast` helper in `gluon/v2/compiler/` that walks a
  `GrammarDescriptor` and emits a canonical schema-AST with the
  kinds this plan's mapping table expects. The proto-sqlite
  pipeline becomes:

      ReadString(sqlite.ebnf)
        → EBNF            → GrammarDescriptor
        → grammar2ast     → ASTDescriptor (schema form)
        → Compile         → FileDescriptorProto

  CST doesn't appear in the lowering path. Future non-EBNF
  languages ship their own `<lang>2ast` helper that emits the
  same canonical AST shape; `Compile` stays unchanged.
- **Should `protomerge://` / `go://` ship with Compile or later?**
  Later — add when a concrete caller shows up. Noted here so
  future-me doesn't re-invent the wiring pattern.
