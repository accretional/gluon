// Package lexkit provides a configurable EBNF lexer and parser that
// reads grammar definitions according to a LexDescriptor configuration.
// It supports Go's EBNF variant, Protocol Buffers' EBNF, and standard
// ISO 14977 EBNF by swapping the LexDescriptor.
package lexkit

import (
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	pb "github.com/accretional/gluon/pb"
	"google.golang.org/protobuf/encoding/prototext"
)

// Char constructs a UTF8 message from a rune. For ASCII range (0-127),
// it uses the ASCII enum. For values >127, it uses the Symbol field.
func Char(r rune) *pb.UTF8 {
	if r >= 0 && r <= 127 {
		return &pb.UTF8{Char: &pb.UTF8_Ascii{Ascii: pb.ASCII(r)}}
	}
	return &pb.UTF8{Char: &pb.UTF8_Symbol{Symbol: uint32(r)}}
}

// RuneOf extracts the rune from a UTF8 message. Returns 0 for nil.
func RuneOf(u *pb.UTF8) rune {
	if u == nil {
		return 0
	}
	switch v := u.Char.(type) {
	case *pb.UTF8_Ascii:
		return rune(v.Ascii)
	case *pb.UTF8_Symbol:
		return rune(v.Symbol)
	}
	return 0
}

// LoadGrammar reads a GrammarDescriptor from a textproto file.
func LoadGrammar(path string) (*pb.GrammarDescriptor, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var gd pb.GrammarDescriptor
	if err := prototext.Unmarshal(data, &gd); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &gd, nil
}

// LoadLex reads a GrammarDescriptor textproto and returns just the
// LexDescriptor from it.
func LoadLex(path string) (*pb.LexDescriptor, error) {
	gd, err := LoadGrammar(path)
	if err != nil {
		return nil, err
	}
	if gd.Lex == nil {
		return nil, fmt.Errorf("%s: no lex descriptor", path)
	}
	return gd.Lex, nil
}

// TokenToRaw converts a TokenDescriptor back to a raw EBNF expression string.
func TokenToRaw(tok *pb.TokenDescriptor) string {
	if tok == nil {
		return ""
	}
	var b strings.Builder
	for _, ch := range tok.Chars {
		b.WriteRune(RuneOf(ch))
	}
	return b.String()
}

// GrammarToParseResult converts a loaded GrammarDescriptor back into a
// ParseResult, enabling re-serialization or re-parsing workflows.
func GrammarToParseResult(gd *pb.GrammarDescriptor) *ParseResult {
	pr := &ParseResult{Lex: gd.Lex}
	for _, pd := range gd.Productions {
		pr.Productions = append(pr.Productions, Production{
			Name: pd.Name,
			Raw:  TokenToRaw(pd.Token),
		})
	}
	return pr
}

// Predefined LexDescriptors for supported EBNF variants.

// GoLex returns the LexDescriptor for Go's EBNF variant.
// Definition: '=', termination: '.', no explicit concatenation,
// terminals in '"' or '`', comments in /* */.
func GoLex() *pb.LexDescriptor {
	return &pb.LexDescriptor{
		Whitespace:    []*pb.UTF8{Char(' '), Char('\t'), Char('\n'), Char('\r')},
		Definition:    Char('='),
		Termination:   Char('.'),
		Alternation:   Char('|'),
		OptionalLhs:   Char('['),
		OptionalRhs:   Char(']'),
		RepetitionLhs: Char('{'),
		RepetitionRhs: Char('}'),
		GroupingLhs:   Char('('),
		GroupingRhs:   Char(')'),
		Terminal:      Char('"'),
		CommentLhs:    Char('/'),
		CommentRhs:    Char('/'),
	}
}

// ProtoLex returns the LexDescriptor for Protocol Buffers' EBNF variant.
// Same as Go except termination is ';' instead of '.'.
func ProtoLex() *pb.LexDescriptor {
	return &pb.LexDescriptor{
		Whitespace:    []*pb.UTF8{Char(' '), Char('\t'), Char('\n'), Char('\r')},
		Definition:    Char('='),
		Termination:   Char(';'),
		Alternation:   Char('|'),
		OptionalLhs:   Char('['),
		OptionalRhs:   Char(']'),
		RepetitionLhs: Char('{'),
		RepetitionRhs: Char('}'),
		GroupingLhs:   Char('('),
		GroupingRhs:   Char(')'),
		Terminal:      Char('"'),
		CommentLhs:    Char('/'),
		CommentRhs:    Char('/'),
	}
}

// StandardLex returns the LexDescriptor for ISO 14977 standard EBNF.
// Uses ',' for concatenation, ';' for termination, (* *) for comments.
func StandardLex() *pb.LexDescriptor {
	return &pb.LexDescriptor{
		Whitespace:    []*pb.UTF8{Char(' '), Char('\t'), Char('\n'), Char('\r')},
		Definition:    Char('='),
		Concatenation: Char(','),
		Termination:   Char(';'),
		Alternation:   Char('|'),
		OptionalLhs:   Char('['),
		OptionalRhs:   Char(']'),
		RepetitionLhs: Char('{'),
		RepetitionRhs: Char('}'),
		GroupingLhs:   Char('('),
		GroupingRhs:   Char(')'),
		Terminal:      Char('"'),
		CommentLhs:    Char('('),
		CommentRhs:    Char(')'),
	}
}

// Production represents a single parsed EBNF production rule.
type Production struct {
	Name string // production name (left-hand side)
	Raw  string // raw EBNF text of the expression (right-hand side)
}

// ParseResult holds the output of parsing an EBNF grammar file.
type ParseResult struct {
	Lex         *pb.LexDescriptor
	Productions []Production
}

// Parse reads EBNF source text and extracts production rules according
// to the given LexDescriptor configuration.
func Parse(src string, lex *pb.LexDescriptor) (*ParseResult, error) {
	p := &parser{
		src: src,
		lex: lex,
		pos: 0,
	}
	prods, err := p.parse()
	if err != nil {
		return nil, err
	}
	return &ParseResult{
		Lex:         lex,
		Productions: prods,
	}, nil
}

// RawToToken converts a raw EBNF expression string into a TokenDescriptor,
// encoding each character as a UTF8 value.
func RawToToken(raw string) *pb.TokenDescriptor {
	var chars []*pb.UTF8
	for _, r := range raw {
		chars = append(chars, Char(r))
	}
	return &pb.TokenDescriptor{Chars: chars}
}

// ToGrammarDescriptor converts a ParseResult into a protobuf GrammarDescriptor.
func (pr *ParseResult) ToGrammarDescriptor() *pb.GrammarDescriptor {
	gd := &pb.GrammarDescriptor{
		Lex:         pr.Lex,
		Productions: make([]*pb.ProductionDescriptor, len(pr.Productions)),
	}
	for i, prod := range pr.Productions {
		gd.Productions[i] = &pb.ProductionDescriptor{
			Name:  prod.Name,
			Token: RawToToken(prod.Raw),
		}
	}
	return gd
}

// ToTextproto serializes a ParseResult as a human-readable textproto
// representation of a GrammarDescriptor.
//
// UTF8 characters in the ASCII range are emitted using ASCII enum names
// (e.g. "ascii: EQUALS_SIGN") rather than raw numbers.
func (pr *ParseResult) ToTextproto() string {
	var b strings.Builder

	b.WriteString("# GrammarDescriptor\n")
	b.WriteString("# Generated by lexkit\n\n")

	// Lex descriptor
	b.WriteString("lex {\n")
	for _, ws := range pr.Lex.Whitespace {
		writeUTF8Field(&b, "whitespace", ws)
	}
	writeUTF8Field(&b, "definition", pr.Lex.Definition)
	writeUTF8Field(&b, "concatenation", pr.Lex.Concatenation)
	writeUTF8Field(&b, "termination", pr.Lex.Termination)
	writeUTF8Field(&b, "alternation", pr.Lex.Alternation)
	writeUTF8Field(&b, "optional_lhs", pr.Lex.OptionalLhs)
	writeUTF8Field(&b, "optional_rhs", pr.Lex.OptionalRhs)
	writeUTF8Field(&b, "repetition_lhs", pr.Lex.RepetitionLhs)
	writeUTF8Field(&b, "repetition_rhs", pr.Lex.RepetitionRhs)
	writeUTF8Field(&b, "grouping_lhs", pr.Lex.GroupingLhs)
	writeUTF8Field(&b, "grouping_rhs", pr.Lex.GroupingRhs)
	writeUTF8Field(&b, "terminal", pr.Lex.Terminal)
	writeUTF8Field(&b, "comment_lhs", pr.Lex.CommentLhs)
	writeUTF8Field(&b, "comment_rhs", pr.Lex.CommentRhs)
	b.WriteString("}\n\n")

	// Productions with name and token descriptor
	fmt.Fprintf(&b, "# %d productions\n", len(pr.Productions))
	for _, prod := range pr.Productions {
		b.WriteString("productions {\n")
		fmt.Fprintf(&b, "  name: %q\n", prod.Name)
		writeTokenDescriptor(&b, prod.Raw)
		b.WriteString("}\n")
	}

	return b.String()
}

// writeTokenDescriptor writes a token descriptor for an EBNF expression,
// encoding each character as a UTF8 value with ASCII enum names.
func writeTokenDescriptor(b *strings.Builder, raw string) {
	b.WriteString("  token {\n")
	for _, r := range raw {
		u := Char(r)
		switch v := u.Char.(type) {
		case *pb.UTF8_Ascii:
			fmt.Fprintf(b, "    chars { ascii: %s }\n", v.Ascii.String())
		case *pb.UTF8_Symbol:
			fmt.Fprintf(b, "    chars { symbol: %d }\n", v.Symbol)
		}
	}
	b.WriteString("  }\n")
}

// writeUTF8Field writes a textproto field for a UTF8 value, using ASCII
// enum names when the character is in the ASCII range.
func writeUTF8Field(b *strings.Builder, name string, u *pb.UTF8) {
	if u == nil {
		return
	}
	switch v := u.Char.(type) {
	case *pb.UTF8_Ascii:
		fmt.Fprintf(b, "  %s { ascii: %s }\n", name, v.Ascii.String())
	case *pb.UTF8_Symbol:
		fmt.Fprintf(b, "  %s { symbol: %d }\n", name, v.Symbol)
	}
}

func truncate(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ") // collapse whitespace
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// parser implements a simple EBNF production-rule extractor.
type parser struct {
	src string
	lex *pb.LexDescriptor
	pos int
}

func (p *parser) parse() ([]Production, error) {
	var prods []Production
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.src) {
			break
		}

		// Look ahead: a new production starts only when we see
		// `identifier <whitespace> = `. If the current position has
		// an identifier followed by something other than '=', it's
		// not a production start (it's a continuation or stray text).
		saved := p.pos
		name, err := p.parseName()
		if err != nil {
			return nil, err
		}
		if name == "" {
			// Not an identifier — skip character
			p.pos++
			continue
		}

		p.skipWhitespaceAndComments()

		// Check for definition operator
		if p.pos >= len(p.src) {
			// EOF after a name with no '=' — not a production
			break
		}
		r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
		if r != RuneOf(p.lex.Definition) {
			// Not a production start — restore position and skip
			// past this identifier (it was part of something else).
			p.pos = saved + len(name)
			continue
		}
		p.pos += sz

		// Read until termination character (not inside quotes/brackets)
		raw, err := p.parseExpression()
		if err != nil {
			return nil, fmt.Errorf("parsing expression for %q: %w", name, err)
		}

		prods = append(prods, Production{
			Name: name,
			Raw:  strings.TrimSpace(raw),
		})
	}
	return prods, nil
}

func (p *parser) parseName() (string, error) {
	p.skipWhitespaceAndComments()
	if p.pos >= len(p.src) {
		return "", nil
	}

	r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
	if !unicode.IsLetter(r) && r != '_' {
		return "", nil
	}

	start := p.pos
	for p.pos < len(p.src) {
		r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			p.pos += sz
		} else {
			break
		}
	}
	return p.src[start:p.pos], nil
}

func (p *parser) parseExpression() (string, error) {
	start := p.pos
	term := RuneOf(p.lex.Termination)
	depth := 0 // track bracket nesting

	for p.pos < len(p.src) {
		r, sz := utf8.DecodeRuneInString(p.src[p.pos:])

		// Handle quoted strings — skip to matching quote
		if r == '"' || r == '\'' || r == '`' {
			p.pos += sz
			p.skipQuoted(r)
			continue
		}

		// Handle comments
		if p.isCommentStart() {
			p.skipComment()
			continue
		}

		// Track bracket depth
		if r == RuneOf(p.lex.GroupingLhs) || r == RuneOf(p.lex.OptionalLhs) || r == RuneOf(p.lex.RepetitionLhs) {
			depth++
		}
		if r == RuneOf(p.lex.GroupingRhs) || r == RuneOf(p.lex.OptionalRhs) || r == RuneOf(p.lex.RepetitionRhs) {
			depth--
		}

		// Termination character at depth 0 ends the production
		if r == term && depth <= 0 {
			raw := p.src[start:p.pos]
			p.pos += sz // consume terminator
			return raw, nil
		}

		p.pos += sz
	}

	// If no termination found, return what we have (some grammars may
	// have the last production without a terminator)
	return p.src[start:p.pos], nil
}

func (p *parser) skipQuoted(quote rune) {
	// EBNF terminal strings are always literal — no escape sequences.
	// The content between quotes represents exact characters in the
	// described language. A backslash inside "\" is a literal backslash,
	// not an escape for the closing quote.
	for p.pos < len(p.src) {
		r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
		p.pos += sz
		if r == quote {
			return
		}
	}
}

func (p *parser) skipWhitespaceAndComments() {
	for p.pos < len(p.src) {
		r, sz := utf8.DecodeRuneInString(p.src[p.pos:])

		// Check whitespace
		isWS := false
		for _, ws := range p.lex.Whitespace {
			if r == RuneOf(ws) {
				isWS = true
				break
			}
		}
		if isWS {
			p.pos += sz
			continue
		}

		// Check for comment start
		if p.isCommentStart() {
			p.skipComment()
			continue
		}

		break
	}
}

func (p *parser) isCommentStart() bool {
	if p.pos >= len(p.src) {
		return false
	}
	clhs := RuneOf(p.lex.CommentLhs)
	if clhs == 0 {
		return false
	}

	r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
	if r != clhs {
		return false
	}

	// Check for // or /* style (when comment_lhs == '/')
	if clhs == '/' && p.pos+1 < len(p.src) {
		next := p.src[p.pos+1]
		return next == '/' || next == '*'
	}

	// Check for (* style (when comment_lhs == '(')
	if clhs == '(' && p.pos+1 < len(p.src) {
		return p.src[p.pos+1] == '*'
	}

	return false
}

func (p *parser) skipComment() {
	clhs := RuneOf(p.lex.CommentLhs)

	if clhs == '/' && p.pos+1 < len(p.src) {
		if p.src[p.pos+1] == '/' {
			// Line comment — skip to end of line
			p.pos += 2
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
			return
		}
		if p.src[p.pos+1] == '*' {
			// Block comment — skip to */
			p.pos += 2
			for p.pos+1 < len(p.src) {
				if p.src[p.pos] == '*' && p.src[p.pos+1] == '/' {
					p.pos += 2
					return
				}
				p.pos++
			}
			p.pos = len(p.src)
			return
		}
	}

	if clhs == '(' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '*' {
		// (* ... *) comment
		p.pos += 2
		for p.pos+1 < len(p.src) {
			if p.src[p.pos] == '*' && p.src[p.pos+1] == ')' {
				p.pos += 2
				return
			}
			p.pos++
		}
		p.pos = len(p.src)
		return
	}

	// Fallback: skip one character
	p.pos++
}
