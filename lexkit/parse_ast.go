package lexkit

// parse_ast.go — Grammar-driven recursive descent parser that takes a
// GrammarDescriptor and source text and produces an ASTDescriptor.
// Each production's EBNF expression is parsed into an Expr tree (via
// expr.go), then matched against the source to build ASTNodeDescriptor
// nodes.
//
// The parser supports configurable options for different languages:
// - Character classes for comment-defined productions
// - Token matchers for lexical productions
// - Left-recursion elimination
// - Keyword boundary matching
// - Lexical/syntactic mode distinction

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	pb "github.com/accretional/gluon/pb"
)

// TokenMatchFunc matches a token starting at pos in src.
// Returns the matched text and new position, or ("", -1) for no match.
type TokenMatchFunc func(src string, pos int) (text string, newPos int)

// ASTParseOptions configures the grammar-driven parser for different languages.
type ASTParseOptions struct {
	// CharClasses maps production names to character-testing functions.
	// Used for comment-defined productions like unicode_char, unicode_letter.
	CharClasses map[string]func(rune) bool

	// TokenMatchers maps production names to functions that match complete
	// tokens from source. When set, the production is matched by calling
	// the function instead of recursing into the grammar.
	TokenMatchers map[string]TokenMatchFunc

	// IsLexical classifies a production as lexical (true) or syntactic.
	// In lexical mode, whitespace is not skipped between elements.
	// If nil, defaults to: names starting with lowercase or '_' are lexical.
	IsLexical func(string) bool

	// Preprocessor transforms source text before parsing.
	// Used for Go-style semicolon insertion.
	Preprocessor func(string) string
}

// DefaultIsLexical returns true for production names starting with a
// lowercase letter or underscore (conventional for lexical productions).
func DefaultIsLexical(name string) bool {
	if len(name) == 0 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsLower(r) || r == '_'
}

// ParseAST parses source text using a GrammarDescriptor and returns an
// ASTDescriptor. Uses EBNF parse options by default for backward
// compatibility. The startRule is the root production to begin parsing
// from (e.g. "Syntax" for EBNF, "SourceFile" for Go).
func ParseAST(src, language, startRule string, gd *pb.GrammarDescriptor) (*pb.ASTDescriptor, error) {
	return ParseASTWithOptions(src, language, startRule, gd, nil)
}

// ParseASTWithOptions parses source text using a GrammarDescriptor with
// configurable options. If opts is nil, EBNF defaults are used.
func ParseASTWithOptions(src, language, startRule string, gd *pb.GrammarDescriptor, opts *ASTParseOptions) (*pb.ASTDescriptor, error) {
	if opts == nil {
		opts = EBNFParseOptions()
	}
	if opts.Preprocessor != nil {
		src = opts.Preprocessor(src)
	}

	ap, err := newASTParser(src, gd, opts)
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
	src       string
	pos       int
	rules     map[string]*Expr // production name -> parsed expression
	lex       *LexConfig
	opts      *ASTParseOptions
	keywords  map[string]bool // alphabetic terminals extracted from grammar
	inLexical bool            // currently inside a lexical production
}

func newASTParser(src string, gd *pb.GrammarDescriptor, opts *ASTParseOptions) (*astParser, error) {
	lc := LexConfigFrom(gd.Lex)

	rules := make(map[string]*Expr, len(gd.Productions))
	for _, prod := range gd.Productions {
		raw := TokenToRaw(prod.Token)
		raw = strings.TrimSpace(raw)
		if raw == "" {
			rules[prod.Name] = nil
			continue
		}
		expr, err := ParseExpr(raw, lc)
		if err != nil {
			return nil, fmt.Errorf("parsing expression for %q: %w", prod.Name, err)
		}
		rules[prod.Name] = expr
	}

	keywords := extractKeywords(rules)

	// Eliminate direct left recursion in all productions.
	for name, expr := range rules {
		if expr != nil {
			rules[name] = eliminateLeftRecursion(name, expr)
		}
	}

	return &astParser{
		src:      src,
		pos:      0,
		rules:    rules,
		lex:      lc,
		opts:     opts,
		keywords: keywords,
	}, nil
}

// extractKeywords finds all alphabetic terminals in the grammar that are
// likely keywords (all-letter, at least 2 characters).
func extractKeywords(rules map[string]*Expr) map[string]bool {
	kw := make(map[string]bool)
	for _, expr := range rules {
		walkExpr(expr, func(e *Expr) {
			if e.Kind == ExprTerminal && len(e.Value) >= 2 && isAllAlpha(e.Value) {
				kw[e.Value] = true
			}
		})
	}
	return kw
}

func walkExpr(e *Expr, fn func(*Expr)) {
	if e == nil {
		return
	}
	fn(e)
	for _, c := range e.Children {
		walkExpr(c, fn)
	}
}

func isAllAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// eliminateLeftRecursion rewrites A = B | A C into A = B { C }.
func eliminateLeftRecursion(name string, expr *Expr) *Expr {
	if expr == nil || expr.Kind != ExprAlternation {
		return expr
	}

	var bases []*Expr
	var suffixes []*Expr

	for _, alt := range expr.Children {
		if isLeftRecursiveAlt(name, alt) {
			suffix := extractRecursiveSuffix(name, alt)
			if suffix != nil {
				suffixes = append(suffixes, suffix)
			}
		} else {
			bases = append(bases, alt)
		}
	}

	if len(suffixes) == 0 || len(bases) == 0 {
		return expr
	}

	var base *Expr
	if len(bases) == 1 {
		base = bases[0]
	} else {
		base = &Expr{Kind: ExprAlternation, Children: bases}
	}

	var suffix *Expr
	if len(suffixes) == 1 {
		suffix = suffixes[0]
	} else {
		suffix = &Expr{Kind: ExprAlternation, Children: suffixes}
	}

	return &Expr{
		Kind: ExprSequence,
		Children: []*Expr{
			base,
			&Expr{Kind: ExprRepetition, Children: []*Expr{suffix}},
		},
	}
}

func isLeftRecursiveAlt(name string, expr *Expr) bool {
	if expr.Kind == ExprNonTerminal && expr.Value == name {
		return true
	}
	if expr.Kind == ExprSequence && len(expr.Children) > 0 {
		first := expr.Children[0]
		return first.Kind == ExprNonTerminal && first.Value == name
	}
	return false
}

func extractRecursiveSuffix(name string, expr *Expr) *Expr {
	if expr.Kind == ExprNonTerminal && expr.Value == name {
		return nil // A = ... | A alone is meaningless
	}
	if expr.Kind == ExprSequence && len(expr.Children) > 1 {
		suffix := expr.Children[1:]
		if len(suffix) == 1 {
			return suffix[0]
		}
		return &Expr{Kind: ExprSequence, Children: suffix}
	}
	return nil
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

// skipWSAndComments skips whitespace and comments. In lexical mode
// (inside a lexical production), nothing is skipped.
func (ap *astParser) skipWSAndComments() {
	if ap.inLexical {
		return
	}
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

// isLexicalProd returns whether a production should be parsed in lexical mode.
func (ap *astParser) isLexicalProd(name string) bool {
	if ap.opts.IsLexical != nil {
		return ap.opts.IsLexical(name)
	}
	return DefaultIsLexical(name)
}

// parseProduction tries to match a named production at the current position.
func (ap *astParser) parseProduction(name string) (*pb.ASTNodeDescriptor, error) {
	expr, ok := ap.rules[name]
	if !ok {
		return nil, fmt.Errorf("unknown production %q", name)
	}
	if expr == nil {
		// Empty production — check character class first.
		if charFn, exists := ap.opts.CharClasses[name]; exists {
			return ap.matchCharClass(name, charFn)
		}
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

	if node.Kind != name {
		node = &pb.ASTNodeDescriptor{
			Kind:     name,
			Children: []*pb.ASTNodeDescriptor{node},
			Location: loc,
		}
	}
	return node, nil
}

// matchExpr dispatches to the correct matcher for an Expr node.
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
	case ExprRange:
		return ap.matchRange(expr.Children[0], expr.Children[1])
	}

	return nil, fmt.Errorf("unknown expr kind %d", expr.Kind)
}

// matchTerminal tries to match a literal string. Applies keyword
// boundary checking: an all-alpha terminal must not be followed by
// a letter, digit, or underscore.
func (ap *astParser) matchTerminal(value string) (*pb.ASTNodeDescriptor, error) {
	ap.skipWSAndComments()
	if ap.pos >= len(ap.src) || !strings.HasPrefix(ap.src[ap.pos:], value) {
		return nil, nil
	}

	// Keyword boundary check
	if len(value) > 0 && isAllAlpha(value) {
		endPos := ap.pos + len(value)
		if endPos < len(ap.src) {
			next, _ := utf8.DecodeRuneInString(ap.src[endPos:])
			if unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_' {
				return nil, nil
			}
		}
	}

	loc := ap.loc()
	ap.pos += len(value)
	return &pb.ASTNodeDescriptor{
		Kind:     "terminal",
		Value:    value,
		Location: loc,
	}, nil
}

// matchRange matches a character range (e.g., "0" … "9").
func (ap *astParser) matchRange(lower, upper *Expr) (*pb.ASTNodeDescriptor, error) {
	if ap.pos >= len(ap.src) {
		return nil, nil
	}
	ch, sz := utf8.DecodeRuneInString(ap.src[ap.pos:])
	lo := firstRune(lower.Value)
	hi := firstRune(upper.Value)
	if lo == 0 || hi == 0 {
		return nil, fmt.Errorf("range bounds must be non-empty")
	}
	if ch >= lo && ch <= hi {
		loc := ap.loc()
		ap.pos += sz
		return &pb.ASTNodeDescriptor{
			Kind:     "terminal",
			Value:    string(ch),
			Location: loc,
		}, nil
	}
	return nil, nil
}

func firstRune(s string) rune {
	if s == "" {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s)
	return r
}

// tryProduction tries to match a production, supporting backtracking.
// Checks token matchers and character classes before grammar recursion.
func (ap *astParser) tryProduction(name string) (*pb.ASTNodeDescriptor, error) {
	// Token matchers take priority.
	if matcher, ok := ap.opts.TokenMatchers[name]; ok {
		return ap.callTokenMatcher(name, matcher)
	}

	// Character classes for comment-defined productions.
	if charFn, ok := ap.opts.CharClasses[name]; ok {
		// Only use the char class when the grammar rule is nil (comment-defined).
		if rule, exists := ap.rules[name]; exists && rule == nil {
			return ap.matchCharClass(name, charFn)
		}
	}

	// Enter lexical mode if this is a lexical production.
	wasLexical := ap.inLexical
	if !ap.inLexical && ap.isLexicalProd(name) {
		ap.inLexical = true
	}

	saved := ap.pos
	node, err := ap.parseProduction(name)
	if err != nil {
		ap.pos = saved
		ap.inLexical = wasLexical
		return nil, nil // backtrack
	}

	ap.inLexical = wasLexical
	return node, nil
}

// callTokenMatcher invokes a registered token matcher.
func (ap *astParser) callTokenMatcher(name string, matcher TokenMatchFunc) (*pb.ASTNodeDescriptor, error) {
	ap.skipWSAndComments()
	text, newPos := matcher(ap.src, ap.pos)
	if newPos < 0 {
		return nil, nil
	}
	loc := ap.loc()
	ap.pos = newPos
	return &pb.ASTNodeDescriptor{
		Kind:     name,
		Value:    text,
		Location: loc,
	}, nil
}

// matchCharClass matches a single character against a class function.
func (ap *astParser) matchCharClass(name string, fn func(rune) bool) (*pb.ASTNodeDescriptor, error) {
	if ap.pos >= len(ap.src) {
		return nil, nil
	}
	ch, sz := utf8.DecodeRuneInString(ap.src[ap.pos:])
	if fn(ch) {
		loc := ap.loc()
		ap.pos += sz
		return &pb.ASTNodeDescriptor{
			Kind:     name,
			Value:    string(ch),
			Location: loc,
		}, nil
	}
	return nil, nil
}

// matchSequence matches all children in order with backtracking.
func (ap *astParser) matchSequence(children []*Expr, context string) (*pb.ASTNodeDescriptor, error) {
	saved := ap.pos
	loc := ap.loc()
	var nodes []*pb.ASTNodeDescriptor

	for _, child := range children {
		node, err := ap.matchExpr(child, context)
		if err != nil {
			ap.pos = saved
			return nil, nil
		}
		if node == nil {
			ap.pos = saved
			return nil, nil
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

// matchAlternation tries each alternative and picks the one that advances
// the position the most (longest match). Empty matches (no advance) are
// used as fallback only if no advancing match exists.
func (ap *astParser) matchAlternation(children []*Expr, context string) (*pb.ASTNodeDescriptor, error) {
	var bestNode *pb.ASTNodeDescriptor
	bestPos := ap.pos
	var emptyMatch *pb.ASTNodeDescriptor
	startPos := ap.pos

	for _, child := range children {
		ap.pos = startPos
		node, err := ap.matchExpr(child, context)
		if err != nil {
			ap.pos = startPos
			continue
		}
		if node != nil {
			if ap.pos > startPos {
				if ap.pos > bestPos {
					bestNode = node
					bestPos = ap.pos
				}
			} else if emptyMatch == nil {
				emptyMatch = node
			}
			ap.pos = startPos
		} else {
			ap.pos = startPos
		}
	}

	if bestNode != nil {
		ap.pos = bestPos
		return bestNode, nil
	}
	return emptyMatch, nil
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
		if ap.pos == saved {
			break // safety: prevent infinite loop
		}
	}

	return &pb.ASTNodeDescriptor{
		Kind:     "repeat",
		Children: nodes,
		Location: loc,
	}, nil
}

func isLetter(ch rune) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isDigit(ch rune) bool {
	return ch >= '0' && ch <= '9'
}
