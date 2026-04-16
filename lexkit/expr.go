package lexkit

// expr.go — Parses EBNF expression bodies (from TokenDescriptor) into
// structured expression trees. These trees drive the grammar-driven
// source parser in parse_ast.go.

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	pb "github.com/accretional/gluon/pb"
)

// ExprKind identifies the type of an EBNF expression node.
type ExprKind int

const (
	ExprSequence    ExprKind = iota // a , b , c (concatenation)
	ExprAlternation                 // a | b | c
	ExprTerminal                    // "literal"
	ExprNonTerminal                 // production_name reference
	ExprOptional                    // [ expr ]
	ExprRepetition                  // { expr }
	ExprGroup                       // ( expr )
	ExprRange                       // "a" … "z" (character range)
)

// Expr is a node in a structured EBNF expression tree.
type Expr struct {
	Kind     ExprKind
	Value    string  // for Terminal: the literal; for NonTerminal: the name
	Children []*Expr // for Sequence, Alternation, Optional, Repetition, Group
}

func (e *Expr) String() string {
	switch e.Kind {
	case ExprTerminal:
		return fmt.Sprintf("%q", e.Value)
	case ExprNonTerminal:
		return e.Value
	case ExprSequence:
		parts := make([]string, len(e.Children))
		for i, c := range e.Children {
			parts[i] = c.String()
		}
		return strings.Join(parts, " , ")
	case ExprAlternation:
		parts := make([]string, len(e.Children))
		for i, c := range e.Children {
			parts[i] = c.String()
		}
		return strings.Join(parts, " | ")
	case ExprOptional:
		return "[ " + e.Children[0].String() + " ]"
	case ExprRepetition:
		return "{ " + e.Children[0].String() + " }"
	case ExprGroup:
		return "( " + e.Children[0].String() + " )"
	case ExprRange:
		return e.Children[0].String() + " … " + e.Children[1].String()
	}
	return "?"
}

// ParseExpr parses an EBNF expression body (the raw string from a
// TokenDescriptor) into a structured Expr tree. The lex parameter
// provides the delimiter characters.
func ParseExpr(raw string, lex *LexConfig) (*Expr, error) {
	p := &exprParser{src: raw, lex: lex, pos: 0}
	expr, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	return expr, nil
}

// LexConfig holds the rune values needed to parse EBNF expressions.
// Extracted from a LexDescriptor for convenience.
type LexConfig struct {
	Alternation   rune
	Concatenation rune // 0 means implicit (juxtaposition)
	OptionalLhs   rune
	OptionalRhs   rune
	RepetitionLhs rune
	RepetitionRhs rune
	GroupingLhs   rune
	GroupingRhs   rune
	Terminal      rune
}

// LexConfigFrom extracts a LexConfig from a proto LexDescriptor.
func LexConfigFrom(lex *pb.LexDescriptor) *LexConfig {
	return &LexConfig{
		Alternation:   RuneOf(lex.Alternation),
		Concatenation: RuneOf(lex.Concatenation),
		OptionalLhs:   RuneOf(lex.OptionalLhs),
		OptionalRhs:   RuneOf(lex.OptionalRhs),
		RepetitionLhs: RuneOf(lex.RepetitionLhs),
		RepetitionRhs: RuneOf(lex.RepetitionRhs),
		GroupingLhs:   RuneOf(lex.GroupingLhs),
		GroupingRhs:   RuneOf(lex.GroupingRhs),
		Terminal:      RuneOf(lex.Terminal),
	}
}

type exprParser struct {
	src string
	lex *LexConfig
	pos int
}

func (p *exprParser) peek() rune {
	if p.pos >= len(p.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
	return r
}

func (p *exprParser) advance() {
	if p.pos < len(p.src) {
		_, sz := utf8.DecodeRuneInString(p.src[p.pos:])
		p.pos += sz
	}
}

func (p *exprParser) skipSpaces() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t' || p.src[p.pos] == '\n' || p.src[p.pos] == '\r') {
		p.pos++
	}
}

// Expression = Term { "|" Term }
func (p *exprParser) parseExpression() (*Expr, error) {
	p.skipSpaces()
	first, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	if first == nil {
		return nil, nil
	}

	var alts []*Expr
	alts = append(alts, first)

	for {
		p.skipSpaces()
		if p.peek() != p.lex.Alternation {
			break
		}
		p.advance() // consume '|'
		term, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		if term == nil {
			return nil, fmt.Errorf("expected term after '|' at pos %d", p.pos)
		}
		alts = append(alts, term)
	}

	if len(alts) == 1 {
		return alts[0], nil
	}
	return &Expr{Kind: ExprAlternation, Children: alts}, nil
}

// Term = Factor { ["," ] Factor }
func (p *exprParser) parseTerm() (*Expr, error) {
	p.skipSpaces()
	first, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	if first == nil {
		return nil, nil
	}

	// Check if first factor is part of a character range
	first = p.maybeRange(first)

	var seq []*Expr
	seq = append(seq, first)

	for {
		p.skipSpaces()
		// Check for end conditions
		ch := p.peek()
		if ch == 0 || ch == p.lex.Alternation || ch == p.lex.GroupingRhs ||
			ch == p.lex.OptionalRhs || ch == p.lex.RepetitionRhs {
			break
		}
		// Skip optional concatenation comma
		if p.lex.Concatenation != 0 && ch == p.lex.Concatenation {
			p.advance()
		}
		factor, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		if factor == nil {
			break
		}
		factor = p.maybeRange(factor)
		seq = append(seq, factor)
	}

	if len(seq) == 1 {
		return seq[0], nil
	}
	return &Expr{Kind: ExprSequence, Children: seq}, nil
}

// maybeRange checks if the given terminal expression is followed by a
// range operator (… U+2026 or ...) and if so, parses the upper bound
// and returns an ExprRange node.
func (p *exprParser) maybeRange(expr *Expr) *Expr {
	if expr == nil || expr.Kind != ExprTerminal {
		return expr
	}
	p.skipSpaces()
	if p.isRangeOp() {
		p.consumeRangeOp()
		p.skipSpaces()
		upper, err := p.parseFactor()
		if err != nil || upper == nil {
			return expr
		}
		return &Expr{Kind: ExprRange, Children: []*Expr{expr, upper}}
	}
	return expr
}

// isRangeOp checks if the current position has a range operator: … (U+2026) or ...
func (p *exprParser) isRangeOp() bool {
	if p.pos >= len(p.src) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(p.src[p.pos:])
	if r == '…' { // U+2026 HORIZONTAL ELLIPSIS
		return true
	}
	if r == '.' && p.pos+2 < len(p.src) && p.src[p.pos+1] == '.' && p.src[p.pos+2] == '.' {
		return true
	}
	return false
}

// consumeRangeOp advances past the range operator.
func (p *exprParser) consumeRangeOp() {
	r, sz := utf8.DecodeRuneInString(p.src[p.pos:])
	if r == '…' {
		p.pos += sz
	} else {
		p.pos += 3 // "..."
	}
}

// Factor = name | terminal | "(" Expression ")" | "[" Expression "]" | "{" Expression "}"
func (p *exprParser) parseFactor() (*Expr, error) {
	p.skipSpaces()
	ch := p.peek()
	if ch == 0 {
		return nil, nil
	}

	// Terminal: "..." or '...' or `...`
	if ch == '"' || ch == '\'' || ch == '`' {
		return p.parseTerminal(ch)
	}

	// Group: ( Expression )
	if ch == p.lex.GroupingLhs {
		p.advance()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.peek() != p.lex.GroupingRhs {
			return nil, fmt.Errorf("expected '%c' at pos %d", p.lex.GroupingRhs, p.pos)
		}
		p.advance()
		return &Expr{Kind: ExprGroup, Children: []*Expr{expr}}, nil
	}

	// Optional: [ Expression ]
	if ch == p.lex.OptionalLhs {
		p.advance()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.peek() != p.lex.OptionalRhs {
			return nil, fmt.Errorf("expected '%c' at pos %d", p.lex.OptionalRhs, p.pos)
		}
		p.advance()
		return &Expr{Kind: ExprOptional, Children: []*Expr{expr}}, nil
	}

	// Repetition: { Expression }
	if ch == p.lex.RepetitionLhs {
		p.advance()
		expr, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if p.peek() != p.lex.RepetitionRhs {
			return nil, fmt.Errorf("expected '%c' at pos %d", p.lex.RepetitionRhs, p.pos)
		}
		p.advance()
		return &Expr{Kind: ExprRepetition, Children: []*Expr{expr}}, nil
	}

	// Non-terminal: identifier
	if unicode.IsLetter(ch) || ch == '_' {
		return p.parseNonTerminal()
	}

	return nil, nil
}

func (p *exprParser) parseTerminal(quote rune) (*Expr, error) {
	p.advance() // skip opening quote
	start := p.pos
	for p.pos < len(p.src) && rune(p.src[p.pos]) != quote {
		p.pos++
	}
	value := p.src[start:p.pos]
	if p.pos < len(p.src) {
		p.advance() // skip closing quote
	}
	return &Expr{Kind: ExprTerminal, Value: value}, nil
}

func (p *exprParser) parseNonTerminal() (*Expr, error) {
	start := p.pos
	for p.pos < len(p.src) {
		ch := rune(p.src[p.pos])
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' {
			p.pos++
		} else {
			break
		}
	}
	return &Expr{Kind: ExprNonTerminal, Value: p.src[start:p.pos]}, nil
}
