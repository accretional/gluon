// Package lexkit provides a configurable EBNF lexer and parser that
// reads grammar definitions according to a LexDescriptor configuration.
// It supports Go's EBNF variant, Protocol Buffers' EBNF, and standard
// ISO 14977 EBNF by swapping the LexDescriptor.
//
// All LexDescriptors are loaded from embedded binary protobuf files.
// Parse results are returned directly as pb.GrammarDescriptor protos.
package lexkit

import (
	_ "embed"
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	pb "github.com/accretional/gluon/pb"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
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

//go:embed ebnf.binarypb
var ebnfLexBytes []byte

//go:embed go.binarypb
var goLexBytes []byte

//go:embed proto.binarypb
var protoLexBytes []byte

func loadLexBytes(name string, data []byte) *pb.LexDescriptor {
	var lex pb.LexDescriptor
	if err := proto.Unmarshal(data, &lex); err != nil {
		panic(fmt.Sprintf("lexkit: corrupt %s: %v", name, err))
	}
	return &lex
}

// EBNFLex returns the LexDescriptor for ISO 14977 standard EBNF.
func EBNFLex() *pb.LexDescriptor { return loadLexBytes("ebnf.binarypb", ebnfLexBytes) }

// GoLex returns the LexDescriptor for Go's EBNF variant.
func GoLex() *pb.LexDescriptor { return loadLexBytes("go.binarypb", goLexBytes) }

// ProtoLex returns the LexDescriptor for Protocol Buffers' EBNF variant.
func ProtoLex() *pb.LexDescriptor { return loadLexBytes("proto.binarypb", protoLexBytes) }

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

// RawToToken converts a raw EBNF expression string into a TokenDescriptor,
// encoding each character as a UTF8 value.
func RawToToken(raw string) *pb.TokenDescriptor {
	var chars []*pb.UTF8
	for _, r := range raw {
		chars = append(chars, Char(r))
	}
	return &pb.TokenDescriptor{Chars: chars}
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

// Parse reads EBNF source text and extracts production rules according
// to the given LexDescriptor configuration. Returns a GrammarDescriptor
// proto directly.
func Parse(src string, lex *pb.LexDescriptor) (*pb.GrammarDescriptor, error) {
	p := &parser{
		src: src,
		lex: lex,
		pos: 0,
	}
	prods, err := p.parse()
	if err != nil {
		return nil, err
	}
	return &pb.GrammarDescriptor{
		Lex:         lex,
		Productions: prods,
	}, nil
}

// ToTextproto serializes a GrammarDescriptor as a human-readable textproto.
// Uses prototext.Marshal which automatically renders ASCII enum names.
func ToTextproto(gd *pb.GrammarDescriptor) string {
	opts := prototext.MarshalOptions{Multiline: true, Indent: "  "}
	out, err := opts.Marshal(gd)
	if err != nil {
		panic(fmt.Sprintf("lexkit: marshal textproto: %v", err))
	}
	return string(out)
}

// parser implements a simple EBNF production-rule extractor.
type parser struct {
	src string
	lex *pb.LexDescriptor
	pos int
}

func (p *parser) parse() ([]*pb.ProductionDescriptor, error) {
	var prods []*pb.ProductionDescriptor
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

		raw = strings.TrimSpace(raw)
		prod := &pb.ProductionDescriptor{
			Name:  name,
			Token: RawToToken(raw),
		}
		if tree, perr := ParseExpr(raw, LexConfigFrom(p.lex)); perr == nil && tree != nil {
			prod.Body = ExprToProductionExpression(tree)
		}
		prods = append(prods, prod)
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
