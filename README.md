# gluon

Self-modifying API service, gRPC Service Linker, and Go coding agent/automation.

## Architecture

Gluon converts Go source code into gRPC services through a multi-stage pipeline,
and is evolving toward a general-purpose transpilation system where protobuf
provides the common structure, tooling, and encoding across all languages.
See [AST_GENERALIZATION_DESIGN.proto](AST_GENERALIZATION_DESIGN.proto) for the
full design vision.

The core logic for all conversions lives in the `codegen/` and `astkit/` packages.
The top-level `X2Y` directories exist as **demonstration and usage example** entry
points for each conversion stage — they do not contain conversion logic themselves.

### Conversion Pipeline

```
Go package ──► AST ──► normalized AST ──► proto messages ──► proto services ──► gRPC server
  pkg2ast       ast2ast        struct2proto     function2service    service2server
```

| Stage | Directory | Core logic | Description |
|-------|-----------|------------|-------------|
| **pkg2ast** | `pkg2ast/` | `codegen.AnalyzeSource`, `codegen.AnalyzeFile` | Parses Go packages into protobuf-encoded representations of their ASTs |
| **ast2ast** | `ast2ast/` | `codegen.TransformInterface` | AST-to-AST manipulation and cross-language fetching (Go ↔ Proto) |
| **struct2proto** | `struct2proto/` | `codegen.GenerateProto`, `goTypeToProto` | Converts Go structs into protobuf message definitions |
| **function2service** | `function2service/` | `codegen.rpcTypes`, `codegen.GenerateMessageDecls` | Wraps function arguments into structs, then maps Go functions to protobuf RPC service definitions |
| **service2server** | `service2server/` | `codegen.Bootstrap`, `codegen.WritePackage` | Generates gRPC server and client implementations from protobuf services, wired to source functions |

### Grammar and Lexical Infrastructure

**Any context-free language can be parsed from a static, serialized
`GrammarDescriptor` in protobuf.** A `GrammarDescriptor` is a superset of
EBNF: it contains a `LexDescriptor` (how to read the notation) and production
rules encoded as character-level `UTF8` token sequences. Deserialize a
`.binarypb` file and call `Parse` — no hardcoded configuration, no code
generation, no language-specific logic.

The system is fully self-hosting: the EBNF grammar is written in EBNF, parsed
by its own `LexDescriptor`, and the resulting `GrammarDescriptor` re-parses
the original source identically. This validates the complete round-trip:

```
EBNF text ──► Parse(src, lex) ──► GrammarDescriptor ──► .binarypb
                                                             |
                                                        ToASCII(gd)
                                                             |
                                                     reconstructed text
                                                             |
                                                    Parse(text, lex)
                                                             |
                                                  identical GrammarDescriptor
```

Three grammars are bootstrapped, each available as source `.txt`, binary
protobuf `.binarypb`, and human-readable `.textproto`:

| Grammar | Productions | Source | binarypb | Validated against |
|---------|-------------|--------|----------|-------------------|
| EBNF | 13 | 1.2 KB | 4.2 KB | Self-hosting round-trip |
| Go | 166 | 9 KB | 28 KB | `golang.org/x/exp/ebnf` (parse + verify) |
| Proto3 | 63 | 3.8 KB | 12 KB | `protoc` / `protoc-gen-go` |

See [`lexkit/README.md`](lexkit/README.md) for the full self-hosting diagram,
serialization format details, and wire format analysis.

**Proto definitions** (the formal type system):

| File | Messages | Purpose |
|------|----------|---------|
| `lex.proto` | `LexDescriptor` | EBNF lexical operators as `UTF8` values |
| `grammar.proto` | `GrammarDescriptor`, `ProductionDescriptor`, `TokenDescriptor` | Complete grammar: lex config + production rules + character tokens |
| `language.proto` | `LanguageDescriptor`, `VersionDescriptor`, `Compiler` | Full language definition (grammar + compiler) |
| `ast.proto` | `ASTDescriptor`, `ASTNodeDescriptor` | Language-agnostic serialized AST (TODO: node fields) |
| `go.proto` | `Go`, `GoMod` services | Go compiler/toolchain as gRPC services |
| `unicode/ascii.proto` | `ASCII` enum | 128 ASCII characters with readable names |
| `unicode/utf_8.proto` | `UTF8` message | `oneof { ASCII ascii, uint32 symbol }` |

### Proto Corpus and Git Infrastructure

Gluon includes tooling to clone repositories, extract `.proto` files, lex them,
and store results organized by proto package — building a dataset for grammar
and transpilation work.

**Proto definitions** (`repos.proto`, `git.proto`):

| File | Messages / Services | Purpose |
|------|---------------------|---------|
| `repos.proto` | `GithubRepo`, `Repo`, `RepoList` | Repository descriptors |
| `git.proto` | `Git` service (`Fetch`, `ListFiles`) | gRPC service for repo operations |

**Packages**:

| Package | Description |
|---------|-------------|
| `gitkit/` | Git repo cloning/updating, proto file scanning, proto source lexing (`LexProto`) |
| `cmd/pull/` | CLI to fetch a repo, lex its `.proto` files, store by package |
| `test/` | Integration tests: lex protos from external repos, validate against corpus |

**Usage** — fetch a repo and index its protos:

```
go run ./cmd/pull/ google/googlesql
go run ./cmd/pull/ -dest ../repos -index ../datasets/protos google/or-tools
```

Flags: `-dest` (clone directory, default `..`), `-index` (proto index directory,
default `../../datasets/protos`), `-shallow` (depth-1 clone).

The test corpus is defined in `test/google_repos.textproto` (57 repos).

### Supporting Packages

| Package | Description |
|---------|-------------|
| `astkit/` | Go AST manipulation library — node construction, traversal, transforms, import management, struct/field utilities (227 tests) |
| `codegen/` | All conversion logic: analysis, transformation, proto generation, server/client bootstrap, proto compilation |
| `lexkit/` | Self-hosting grammar toolkit — parses any context-free language from serialized `GrammarDescriptor` protobuf |
| `gitkit/` | Git repo management, proto file discovery, and proto source lexing |
| `pb/` | Generated protobuf Go code from all `.proto` files |
| `cmd/` | CLI entry points: `gluon-server` (codegen subcommand), `pull` (repo fetch + proto index) |

### Examples

The `examples/` directory is a repository of different kinds of types, packages,
and other Go/protobuf entities used for **testing and validation** of the
conversion pipeline. It provides diverse inputs that exercise edge cases across
all stages:

| Directory | Purpose |
|-----------|---------|
| `examples/structs/` | Struct definitions organized by complexity |
| `examples/structs/simple.go` | Structs using only non-pointer predeclared base types |
| `examples/structs/high-dep/` | Structs with many external package dependencies |
| `examples/structs/pointers-and-memory/` | Pointer types, unsafe patterns, memory layout |
| `examples/structs/os-and-complexity/` | OS-level types, generics, complex nesting |

### Go Spec Reference

The `go-spec/` directory caches and fragments the Go language specification for
reference. Run the extractor to split the raw spec into per-section HTML files:

```
go run ./go-spec/cmd/extract/ go-spec/raw_1_26.html
```

### Generating Grammars

To regenerate all grammar outputs (textproto, binarypb, reconstructed text):

```
go run ./lexkit/cmd/lexgen/ lexkit/
```

This parses all three EBNF grammars, writes `.textproto`, `.binarypb`, and
`_reconstructed.txt` for each, validates Go against `x/exp/ebnf`, validates
EBNF self-hosting, and confirms the round-trip.
