# Notes from a first-time external consumer (proto-ip)

These notes were written while integrating gluon v2 into a sibling
repo (`accretional/proto-ip`) that defines IPv4 / IPv6 / CIDR EBNF
grammars and asks gluon to validate strings against them. Writing
them down because there were several non-obvious gotchas that cost
real time, and the gluon maintainer asked for honest feedback.

## What this commit changes

`lexkit.ParseAST` now requires the start rule to consume the whole
input. Trailing whitespace and comments are still skipped (matching
the behavior `lexkit.Parse` already has for EBNF source); anything
else past the last terminal is a parse error.

- `lexkit/parse_ast.go`: 6-line check at the end of
  `ParseASTWithOptions`. No new option, no opt-out — full consumption
  is the universal expectation for "the grammar accepted my input."
- `lexkit/parse_ast_eof_test.go`: new test, 5 cases (clean, trailing
  junk, trailing dot, prefix only, trailing whitespace).
- All existing tests still pass (`go test ./...`).

The single non-test caller of `lexkit.ParseAST` is
`metaparser.ParseCST`, so the change propagates to v2 automatically.
v2's `ParseCST` is now strict by default at no source change.

## Why this needed to happen

Before the patch, `ParseAST("1.2.3.4junk", ipv4Grammar)` returned
success. The CST AST covered just the `1.2.3.4` prefix; the trailing
`junk` was silently dropped. For any caller doing
"is this string valid against my grammar?" — which is the natural
shape of validation — that's a bug masquerading as a feature.

The proto-ip grammars depend on this fix to be the spec: every
range / count / shape constraint is expressed in the .ebnf file, and
the test harness asks gluon "did you accept it?" with no Go-side
post-validation. Without the EOF check, "256.0.0.1" would be
accepted because gluon would consume "25" as the first octet and
silently ignore "6.0.0.1".

## Things I struggled with (calibrated for the next external consumer)

### 1. Knowing which package to import

gluon has multiple parser surfaces:

- `lexkit` (v1)
- `metaparser` (v1, deprecated)
- `v2/metaparser` (current public surface)
- `v2/compiler` (AST → FileDescriptorProto)
- `v2/astkit` (tree ops)

The v2 README is good but it took several reads to internalise that
`v2/metaparser.ParseEBNF` + `v2/metaparser.ParseCST` is the validate
flow, and that I should never import `lexkit` directly from a v2
consumer. My first pass did import `lexkit`; correcting that
required a second editing round.

**Suggestion:** in the v2 README, add a "When to import what" matrix
near the top. Something like:

| You want to | Import |
|---|---|
| Parse EBNF source → grammar | `v2/metaparser` |
| Validate input against a grammar | `v2/metaparser` |
| Lower an AST to proto descriptors | `v2/compiler` |
| Tree-walk / rewrite an AST | `v2/astkit` |
| Anything in v1 (`lexkit`, top-level `metaparser`) | Don't, unless you're patching gluon itself |

### 2. The "first rule is the start rule" convention

`ParseCST` uses `gd.GetRules()[0].GetName()` as the start rule. This
is reasonable but undocumented in `metaparser.proto` — the doc says
"parses an already-lexed TokenSequence against a grammar" without
calling out the convention. Took some grep-archaeology to find it
in `cst.go` and then refactor my grammar files to put the entry
rule first.

**Suggestion:** document this in `CstRequest`'s comment in
`metaparser.proto`.

### 3. `matchTerminal`'s keyword-boundary check for single-char alpha terminals

This one bit me hard. Given a grammar like:

```ebnf
hex_digit = "0" | "1" | ... | "9" | "a" | "b" | ... | "f" ;
h16       = hex_digit , [ hex_digit , [ hex_digit , [ hex_digit ] ] ] ;
```

matching "ab" as `h16` (i.e., two `hex_digit`s) **silently fails**.
Reason: `matchTerminal` applies a keyword-boundary check that any
all-alpha terminal followed by a letter / digit / underscore is
rejected — `"a"` matches at pos 0, but pos 1 is `"b"` (alpha), so
the match is voided. The check is there to stop `"for"` from
matching inside `"former"`, which is right. But it generalises
incorrectly to single-char alpha terminals in lexical productions.

The fix is to use a range instead of enumeration:

```ebnf
hex_digit = digit | "a" ... "f" | "A" ... "F" ;
```

Ranges go through `matchRange`, which has no keyword-boundary check.

This took me a long time to spot because the grammar still parses
*some* inputs (anything with non-alpha following a hex char), so the
failure mode is "accepts some valid inputs, silently rejects others"
rather than a clean parse error.

**Suggestions:**

- Document this in `matchTerminal` and in the EBNF authoring guide
  ("if you're enumerating a single-character class, prefer ranges").
- Or: gate the keyword-boundary check on terminal length >= 2. Any
  realistic keyword is at least two characters; one-character alpha
  literals don't form keywords. This would have caught my case
  without changing behavior for actual SQL/Go keyword grammars.
- Or: skip the keyword-boundary check inside lexical productions
  (`inLexical == true`). The check exists to disambiguate keywords
  from identifiers in *syntactic* mode; lexical productions don't
  have identifiers.

I think the third option is the cleanest. The check is currently
applied unconditionally in `matchTerminal`, even when `inLexical`.

### 4. `matchOptional` does not backtrack across "match something" / "match nothing"

The RFC 4291 IPv6 grammar uses `[ *N( h16 ":" ) h16 ] "::" ...` to
mean "0..N+1 h16's before `::`". Translated naively to gluon EBNF:

```ebnf
zero_to_6_h16c = [ h16 , ":" , zero_to_5_h16c ] ;
form9 = [ zero_to_6_h16c , h16 ] , "::" ;
```

This **does not** match `"1::"`, even though it should. Trace:

1. Outer `[ zero_to_6_h16c , h16 ]` enters its inner sequence.
2. `zero_to_6_h16c` (an optional) tries its inner — matches `"1:"`,
   advances pos to 2.
3. The outer sequence then needs `h16`, but pos 2 is `":"`. Fail.
4. Sequence resets pos to 0, returns nil.
5. The outer optional sees nil from its inner and yields empty.
6. Back at form9: `"::"` is now expected at pos 0, which is `"1"`. Fail.

The problem: `matchOptional` commits to the `"match something"`
branch as soon as the inner sequence partially advances. Once the
sequence later collapses, the optional just yields empty — it
doesn't retry the inner with "match nothing" semantics. Standard
RFC ABNF assumes backtracking; gluon's PEG-style optional doesn't.

The proto-ip workaround is to enumerate: index by `K` (the count
of h16's before `::`) so `matchAlternation`'s longest-match picks
the right K. Each K becomes a separate top-level alternative with
no greedy optional inside.

This is workable but it converts a 9-rule RFC translation into a
30+ rule grammar and pushes meaningful authoring complexity onto
the grammar writer.

**Suggestions:**

- Document this clearly in EBNF authoring guidance —
  "gluon's optional/repetition do not backtrack; structure your
  grammar to use alternation for ambiguous prefix/suffix splits."
- Longer term: consider true backtracking in `matchOptional` —
  if the surrounding sequence later fails after the optional
  matched non-empty, retry with empty. This could be implemented
  without changing the public API (sequence-level rollback).

### 5. v1 / v2 boundary

`v2/metaparser/cst.go` calls `lexkit.ParseAST` directly — v2's CST
*is* a v1 parser shim with translation layers on either side. The
v2 README acknowledges this. It means:

- Patches to v2 parser behavior have to land in v1's `lexkit`.
- The v2 grammar messages are translated back to EBNF text via
  `printExpressions`, then re-parsed by v1's EBNF lexer in
  `convertGrammarToV1`. This is a brittle round-trip — any
  serialization/escaping bug in `printExpressions` would corrupt
  the grammar silently.

This is fine while the shim exists, but is worth flagging for
anyone reading v2 thinking it's a clean rewrite. It isn't (yet).

The v2 README explicitly tracks "Native v2 parser" as open work.
Keeping that flagged.

## What I'd do differently next time

1. Read all of `lexkit/parse_ast.go` plus all of `v2/metaparser/`
   before writing any grammar. The gotchas above are all visible
   in the source if you go looking; I went looking too late.
2. Build the smallest possible grammar test harness FIRST (one
   `.ebnf` + one bufconn-based test, two corpus entries), then
   iterate on grammars. Saves time vs. writing a big grammar and
   debugging by inference.
3. Use ranges (`"0" ... "9"`) by default for single-character
   classes; only use enumeration (`"a" | "b" | ...`) when the
   alternatives are multi-character.
4. Treat `matchAlternation`'s longest-match as the only reliable
   backtracking mechanism, and structure the grammar around it.
   Don't reach for `[...]` and `{...}` to express ambiguity.

## What still needs addressing (gluon-side)

In rough priority:

1. **Document the keyword-boundary gotcha** (or, better, scope it
   to syntactic-mode-only / multi-char terminals). This is the
   gotcha most likely to silently bite the next consumer.
2. **Document the "first rule is start rule" convention** in
   `metaparser.proto`'s `CstRequest` doc.
3. **Document the no-backtracking-in-optional/repetition behavior**
   somewhere visible — ideally with the IPv6-style enumeration
   pattern as a worked example.
4. **Add a `When to import what` matrix to the v2 README** —
   reduces the chance of a v2 consumer reaching into `lexkit`.
5. **Native v2 parser** (already tracked). Removes the v1 shim and
   the brittle `printExpressions` round-trip.

— integration done by Claude (proto-ip session, 2026-04-29)
