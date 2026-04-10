# Debugging binarypb with protoscope

[protoscope](https://github.com/protocolbuffers/protoscope) is a tool for
inspecting and authoring protobuf wire format. It decodes binary protobuf
into a human-readable text format showing field numbers, wire types, and
values — without needing the .proto schema.

## Install

```sh
go install github.com/protocolbuffers/protoscope/cmd/protoscope@latest
```

Binary lands in `~/go/bin/protoscope` (or `$GOBIN/protoscope`).

## Basic usage

```sh
# Decode a binarypb file to protoscope text
protoscope some_file.binarypb

# Encode protoscope text back to binary
protoscope -s < some_file.textscope > some_file.binarypb

# Pipe from stdin
cat some_file.binarypb | protoscope
```

## Reading protoscope output

The output uses field numbers (not names) since protoscope doesn't know
the schema. Cross-reference with the .proto definition:

```
# grammar.proto field numbers:
# GrammarDescriptor: 1=lex, 2=productions
# ProductionDescriptor: 1=name, 2=token
# TokenDescriptor: 1=chars (repeated)
# UTF8 (oneof char): 1=ascii (enum), 2=symbol (uint32)
# LexDescriptor: 1=whitespace, 2=definition, 3=concatenation, ...
```

Example output for `ebnf_grammar.binarypb`:

```
1: {                    # lex (LexDescriptor)
  1: {1: 32}            #   whitespace: { ascii: SPACE (32) }
  1: {1: 9}             #   whitespace: { ascii: CHARACTER_TABULATION (9) }
  2: {1: 61}            #   definition: { ascii: EQUALS_SIGN (61) }
  ...
}
2: {                    # productions[0] (ProductionDescriptor)
  1: {"Syntax"}         #   name: "Syntax"
  2: {                  #   token (TokenDescriptor)
    1: {1: 123}         #     chars: { ascii: LEFT_CURLY_BRACKET (123) }
    1: {1: 32}          #     chars: { ascii: SPACE (32) }
    1: {1: 80}          #     chars: { ascii: LATIN_CAPITAL_LETTER_P (80) }
    ...
  }
}
```

## Size analysis

Each character in a TokenDescriptor is encoded as a nested UTF8 message:

```
TokenDescriptor.chars (field 1, LEN) ->
  UTF8.ascii (field 1, VARINT) -> enum value
```

Wire format per character:
- 1 byte: tag for `chars` (field 1, wire type LEN)
- 1 byte: length of UTF8 submessage (typically 2)
- 1 byte: tag for `ascii` (field 1, wire type VARINT)
- 1 byte: enum value (for ASCII 0-127)
= **4 bytes per character** (vs 1 byte raw)

This means the binarypb is ~4x larger than the raw text content:

```
$ go run /tmp/analyze.go
  EBNF:   992 raw chars ->  4,228 bytes binarypb (4.3x)
  Go:   ~7000 raw chars -> 27,625 bytes binarypb (~4x)
  Proto: ~3000 raw chars -> 12,371 bytes binarypb (~4x)
```

The overhead comes from wrapping every character in a `UTF8 { ascii: N }`
message. This is the cost of using proto-symbol's character-level encoding
for readable textproto output. A `bytes` field would be 1:1 but lose the
ASCII enum names in textproto.

## Useful commands

```sh
# Quick size check
wc -c lexkit/*.binarypb

# Count characters in a grammar
protoscope lexkit/ebnf_grammar.binarypb | grep -c '1: {1:'

# Look at just the lex descriptor (field 1)
protoscope lexkit/ebnf_grammar.binarypb | sed -n '1,/^}/p'

# Look at a specific production (search by name)
protoscope lexkit/ebnf_grammar.binarypb | grep -A 20 '"Syntax"'
```
