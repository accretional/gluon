package lexkit

// parse_ast.go — Grammar-driven recursive descent parser that takes a
// GrammarDescriptor and source text and produces an ASTDescriptor.
// Each production's EBNF expression is parsed into an Expr tree (via
// expr.go), then matched against the source to build ASTNodeDescriptor
// nodes.

import (
	"fmt"
	"strings"

	pb "github.com/accretional/gluon/pb"
)

// ParseAST parses source text using a GrammarDescriptor and returns an
// ASTDescriptor. The startRule is the root production to begin parsing
// from (e.g. "Syntax" for EBNF, "SourceFile" for Go).
func ParseAST(src, language, startRule string, gd *pb.GrammarDescriptor) (*pb.ASTDescriptor, error) {
	ap, err := newASTParser(src, gd)
	if err != nil {
		return nil, err
	}

	root, err := ap.parseProduction(startRule)
	if err != nil {
		return nil, fmt.Errorf("parsing %q: %w", startRule, err)
	}

	return &pb.ASTDescriptor{
		Language: language,
		Root:     root,
	}, nil
}

// astParser holds the state for grammar-driven parsing.
type astParser struct {
	src   string
	pos   int
	rules map[string]*Expr // production name -> parsed expression
	lex   *LexConfig
}

func newASTParser(src string, gd *pb.GrammarDescriptor) (*astParser, error) {
	lc := LexConfigFrom(gd.Lex)

	rules := make(map[string]*Expr, len(gd.Productions))
	for _, prod := range gd.Productions {
		raw := TokenToRaw(prod.Token)
		expr, err := ParseExpr(raw, lc)
		if err != nil {
			return nil, fmt.Errorf("parsing expression for %q: %w", prod.Name, err)
		}
		rules[prod.Name] = expr
	}

	return &astParser{
		src:   src,
		pos:   0,
		rules: rules,
		lex:   lc,
	}, nil
}

// loc returns a SourceLocation for the current position.
func (ap *astParser) loc() *pb.SourceLocation {
	line := 1
	col := 1
	for i := 0; i < ap.pos && i < len(ap.src); i++ {
		if ap.src[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return &pb.SourceLocation{
		Offset: int32(ap.pos),
		Line:   int32(line),
		Column: int32(col),
	}
}

// skipWSAndComments skips whitespace and comments in the source.
// For now, handles // and /* */ comments (common to most languages).
func (ap *astParser) skipWSAndComments() {
	for ap.pos < len(ap.src) {
		ch := ap.src[ap.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			ap.pos++
			continue
		}
		// // line comment
		if ch == '/' && ap.pos+1 < len(ap.src) && ap.src[ap.pos+1] == '/' {
			ap.pos += 2
			for ap.pos < len(ap.src) && ap.src[ap.pos] != '\n' {
				ap.pos++
			}
			continue
		}
		// /* block comment */
		if ch == '/' && ap.pos+1 < len(ap.src) && ap.src[ap.pos+1] == '*' {
			ap.pos += 2
			for ap.pos+1 < len(ap.src) {
				if ap.src[ap.pos] == '*' && ap.src[ap.pos+1] == '/' {
					ap.pos += 2
					break
				}
				ap.pos++
			}
			continue
		}
		// (* block comment *)
		if ch == '(' && ap.pos+1 < len(ap.src) && ap.src[ap.pos+1] == '*' {
			ap.pos += 2
			for ap.pos+1 < len(ap.src) {
				if ap.src[ap.pos] == '*' && ap.src[ap.pos+1] == ')' {
					ap.pos += 2
					break
				}
				ap.pos++
			}
			continue
		}
		break
	}
}

// parseProduction tries to match a named production at the current position.
func (ap *astParser) parseProduction(name string) (*pb.ASTNodeDescriptor, error) {
	expr, ok := ap.rules[name]
	if !ok {
		return nil, fmt.Errorf("unknown production %q", name)
	}
	if expr == nil {
		return &pb.ASTNodeDescriptor{Kind: name, Location: ap.loc()}, nil
	}

	loc := ap.loc()
	node, err := ap.matchExpr(expr, name)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, fmt.Errorf("production %q did not match at offset %d", name, ap.pos)
	}

	// Wrap in a production node if the match result is not already one
	if node.Kind != name {
		node = &pb.ASTNodeDescriptor{
			Kind:     name,
			Children: []*pb.ASTNodeDescriptor{node},
			Location: loc,
		}
	}
	return node, nil
}

// matchExpr tries to match an Expr against the source at the current position.
// Returns nil if no match (for backtracking). Returns error for structural failures.
func (ap *astParser) matchExpr(expr *Expr, context string) (*pb.ASTNodeDescriptor, error) {
	if expr == nil {
		return nil, nil
	}

	switch expr.Kind {
	case ExprTerminal:
		return ap.matchTerminal(expr.Value)

	case ExprNonTerminal:
		return ap.tryProduction(expr.Value)

	case ExprSequence:
		return ap.matchSequence(expr.Children, context)

	case ExprAlternation:
		return ap.matchAlternation(expr.Children, context)

	case ExprOptional:
		return ap.matchOptional(expr.Children[0], context)

	case ExprRepetition:
		return ap.matchRepetition(expr.Children[0], context)

	case ExprGroup:
		return ap.matchExpr(expr.Children[0], context)
	}

	return nil, fmt.Errorf("unknown expr kind %d", expr.Kind)
}

// matchTerminal tries to match a literal string at the current position.
func (ap *astParser) matchTerminal(value string) (*pb.ASTNodeDescriptor, error) {
	ap.skipWSAndComments()
	if strings.HasPrefix(ap.src[ap.pos:], value) {
		loc := ap.loc()
		ap.pos += len(value)
		return &pb.ASTNodeDescriptor{
			Kind:     "terminal",
			Value:    value,
			Location: loc,
		}, nil
	}
	return nil, nil
}

// tryProduction tries to match a production, supporting backtracking.
// Lexical productions (terminal, letter, digit, character, production_name)
// are handled with specialized matchers for correctness and performance.
func (ap *astParser) tryProduction(name string) (*pb.ASTNodeDescriptor, error) {
	// Lexical productions: match directly instead of recursing through
	// character-level grammar rules. This handles edge cases (quotes
	// inside quotes) and avoids O(n * alphabet) per character.
	switch name {
	case "terminal":
		return ap.matchQuotedTerminal()
	case "production_name":
		return ap.matchIdentifier()
	case "letter":
		return ap.matchLetter()
	case "digit":
		return ap.matchDigit()
	case "character":
		return ap.matchCharacter()
	}

	saved := ap.pos
	node, err := ap.parseProduction(name)
	if err != nil {
		ap.pos = saved
		return nil, nil // backtrack, not a hard error
	}
	return node, nil
}

// matchQuotedTerminal matches a quoted string: '...' or "..."
func (ap *astParser) matchQuotedTerminal() (*pb.ASTNodeDescriptor, error) {
	ap.skipWSAndComments()
	if ap.pos >= len(ap.src) {
		return nil, nil
	}
	ch := ap.src[ap.pos]
	if ch != '\'' && ch != '"' {
		return nil, nil
	}
	loc := ap.loc()
	ap.pos++ // skip opening quote
	start := ap.pos
	for ap.pos < len(ap.src) && ap.src[ap.pos] != ch {
		ap.pos++
	}
	value := ap.src[start:ap.pos]
	if ap.pos < len(ap.src) {
		ap.pos++ // skip closing quote
	}
	return &pb.ASTNodeDescriptor{
		Kind:     "terminal",
		Value:    value,
		Location: loc,
	}, nil
}

// matchIdentifier matches a production name: letter { letter | digit | '_' }
func (ap *astParser) matchIdentifier() (*pb.ASTNodeDescriptor, error) {
	ap.skipWSAndComments()
	if ap.pos >= len(ap.src) {
		return nil, nil
	}
	ch := rune(ap.src[ap.pos])
	if !isLetter(ch) {
		return nil, nil
	}
	loc := ap.loc()
	start := ap.pos
	for ap.pos < len(ap.src) {
		c := rune(ap.src[ap.pos])
		if isLetter(c) || isDigit(c) || c == '_' {
			ap.pos++
		} else {
			break
		}
	}
	return &pb.ASTNodeDescriptor{
		Kind:     "production_name",
		Value:    ap.src[start:ap.pos],
		Location: loc,
	}, nil
}

// matchLetter matches a single ASCII letter.
func (ap *astParser) matchLetter() (*pb.ASTNodeDescriptor, error) {
	ap.skipWSAndComments()
	if ap.pos >= len(ap.src) {
		return nil, nil
	}
	ch := rune(ap.src[ap.pos])
	if !isLetter(ch) {
		return nil, nil
	}
	loc := ap.loc()
	ap.pos++
	return &pb.ASTNodeDescriptor{
		Kind:     "letter",
		Value:    string(ch),
		Location: loc,
	}, nil
}

// matchDigit matches a single ASCII digit.
func (ap *astParser) matchDigit() (*pb.ASTNodeDescriptor, error) {
	ap.skipWSAndComments()
	if ap.pos >= len(ap.src) {
		return nil, nil
	}
	ch := rune(ap.src[ap.pos])
	if !isDigit(ch) {
		return nil, nil
	}
	loc := ap.loc()
	ap.pos++
	return &pb.ASTNodeDescriptor{
		Kind:     "digit",
		Value:    string(ch),
		Location: loc,
	}, nil
}

// matchCharacter matches any printable ASCII character (for EBNF).
func (ap *astParser) matchCharacter() (*pb.ASTNodeDescriptor, error) {
	ap.skipWSAndComments()
	if ap.pos >= len(ap.src) {
		return nil, nil
	}
	ch := rune(ap.src[ap.pos])
	if ch >= ' ' && ch <= '~' {
		loc := ap.loc()
		ap.pos++
		return &pb.ASTNodeDescriptor{
			Kind:     "character",
			Value:    string(ch),
			Location: loc,
		}, nil
	}
	return nil, nil
}

func isLetter(ch rune) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}

// matchSequence matches all children in order.
func (ap *astParser) matchSequence(children []*Expr, context string) (*pb.ASTNodeDescriptor, error) {
	saved := ap.pos
	loc := ap.loc()
	var nodes []*pb.ASTNodeDescriptor

	for _, child := range children {
		node, err := ap.matchExpr(child, context)
		if err != nil {
			ap.pos = saved
			return nil, nil // backtrack
		}
		if node == nil {
			ap.pos = saved
			return nil, nil // backtrack
		}
		nodes = append(nodes, node)
	}

	if len(nodes) == 1 {
		return nodes[0], nil
	}
	return &pb.ASTNodeDescriptor{
		Kind:     context,
		Children: nodes,
		Location: loc,
	}, nil
}

// matchAlternation tries each alternative, returns the first match.
func (ap *astParser) matchAlternation(children []*Expr, context string) (*pb.ASTNodeDescriptor, error) {
	for _, child := range children {
		saved := ap.pos
		node, err := ap.matchExpr(child, context)
		if err != nil {
			ap.pos = saved
			continue
		}
		if node != nil {
			return node, nil
		}
		ap.pos = saved
	}
	return nil, nil
}

// matchOptional tries to match; returns empty node if no match.
func (ap *astParser) matchOptional(expr *Expr, context string) (*pb.ASTNodeDescriptor, error) {
	saved := ap.pos
	node, err := ap.matchExpr(expr, context)
	if err != nil || node == nil {
		ap.pos = saved
		return &pb.ASTNodeDescriptor{Kind: "optional", Location: ap.loc()}, nil
	}
	return node, nil
}

// matchRepetition matches zero or more occurrences.
func (ap *astParser) matchRepetition(expr *Expr, context string) (*pb.ASTNodeDescriptor, error) {
	loc := ap.loc()
	var nodes []*pb.ASTNodeDescriptor

	for {
		saved := ap.pos
		node, err := ap.matchExpr(expr, context)
		if err != nil || node == nil {
			ap.pos = saved
			break
		}
		nodes = append(nodes, node)
		// Safety: if we didn't advance, stop to prevent infinite loop
		if ap.pos == saved {
			break
		}
	}

	return &pb.ASTNodeDescriptor{
		Kind:     "repeat",
		Children: nodes,
		Location: loc,
	}, nil
}
