package metaparser

// render_ebnf.go — the inverse direction of ParseEBNF + GrammarToAST: render a
// compiler ASTDescriptor back to EBNF text. All concrete punctuation is read
// from a LexDescriptor (default: the canonical ISO-14977 table, EBNFLexV2), so
// the renderer is syntax-agnostic — nothing about EBNF's glyphs is hardcoded —
// and it round-trips: for a grammar expressible in lex, re-parsing the output
// yields a structurally equal AST. Addresses accretional/gluon#10.
//
// Most gluon work parses input data rather than generating grammars, so this is
// a plain package export (no service), used by descriptor-first tools that build
// an AST programmatically and want a human-readable grammar view.

import (
	"fmt"
	"strings"

	"github.com/accretional/gluon/v2/compiler"
	pb "github.com/accretional/gluon/v2/pb"
)

// ebnfGlyphs is the concrete punctuation resolved from a LexDescriptor.
type ebnfGlyphs struct {
	def, term, concat, alt string
	optOpen, optClose      string
	repOpen, repClose      string
	grpOpen, grpClose      string
	termOpen, termClose    string
	comOpen, comClose      string
}

func glyphsFromLex(lex *pb.LexDescriptor) ebnfGlyphs {
	// ISO-14977 fallbacks; overridden by whatever lex actually specifies.
	g := ebnfGlyphs{
		def: "=", term: ";", concat: ",", alt: "|",
		optOpen: "[", optClose: "]", repOpen: "{", repClose: "}",
		grpOpen: "(", grpClose: ")", termOpen: `"`, termClose: `"`,
		comOpen: "(*", comClose: "*)",
	}
	for _, s := range lex.GetSymbols() {
		if d := s.GetDelimiter(); d != nil {
			switch d.GetKind() {
			case pb.Delimiter_DEFINITION:
				g.def = d.GetSymbol()
			case pb.Delimiter_TERMINATION:
				g.term = d.GetSymbol()
			case pb.Delimiter_CONCATENATION:
				g.concat = d.GetSymbol()
			case pb.Delimiter_ALTERNATION:
				g.alt = d.GetSymbol()
			}
		}
		if sc := s.GetScoper(); sc != nil {
			switch sc.GetKind() {
			case pb.Scoper_OPTIONAL:
				g.optOpen, g.optClose = sc.GetBegin(), sc.GetEnd()
			case pb.Scoper_REPETITION:
				g.repOpen, g.repClose = sc.GetBegin(), sc.GetEnd()
			case pb.Scoper_GROUP:
				g.grpOpen, g.grpClose = sc.GetBegin(), sc.GetEnd()
			case pb.Scoper_TERMINAL:
				g.termOpen, g.termClose = sc.GetBegin(), sc.GetEnd()
			case pb.Scoper_COMMENT:
				g.comOpen, g.comClose = sc.GetBegin(), sc.GetEnd()
			}
		}
	}
	return g
}

func (g ebnfGlyphs) comment(s string) string { return g.comOpen + " " + s + " " + g.comClose }

// RenderEBNF renders a compiler ASTDescriptor (see gluon/v2/compiler for the
// AST kind conventions) as EBNF text. lex supplies the concrete punctuation; nil
// means the canonical ISO-14977 table (EBNFLexV2). annotate, when non-nil, is
// called with each rule name and may return a comment to emit on the line above
// that rule (return "" for none) — this lets descriptor-first callers attach
// semantics (e.g. a subClassOf note) without any string manipulation of the
// output.
func RenderEBNF(ast *pb.ASTDescriptor, lex *pb.LexDescriptor, annotate func(rule string) string) (string, error) {
	if ast == nil || ast.GetRoot() == nil {
		return "", fmt.Errorf("RenderEBNF: nil AST")
	}
	root := ast.GetRoot()
	if root.GetKind() != compiler.KindFile {
		return "", fmt.Errorf("RenderEBNF: AST root kind %q, want %q", root.GetKind(), compiler.KindFile)
	}
	if lex == nil {
		lex = EBNFLexV2()
	}
	g := glyphsFromLex(lex)

	var b strings.Builder
	for _, rule := range root.GetChildren() {
		if rule.GetKind() != compiler.KindRule {
			return "", fmt.Errorf("RenderEBNF: file child kind %q, want %q", rule.GetKind(), compiler.KindRule)
		}
		if annotate != nil {
			if note := annotate(rule.GetValue()); note != "" {
				b.WriteString(g.comment(note) + "\n")
			}
		}
		body := ""
		if kids := rule.GetChildren(); len(kids) == 1 {
			s, err := renderNode(kids[0], 4, g)
			if err != nil {
				return "", fmt.Errorf("RenderEBNF: rule %q: %w", rule.GetValue(), err)
			}
			body = s
		}
		fmt.Fprintf(&b, "%s %s %s %s\n\n", rule.GetValue(), g.def, body, g.term)
	}
	return b.String(), nil
}

// renderNode renders one AST expression node. indent is the column at which
// wrapped sequence/alternation items are placed.
func renderNode(n *pb.ASTNode, indent int, g ebnfGlyphs) (string, error) {
	pad := "\n" + strings.Repeat(" ", indent)
	joinWrapped := func(op string, kids []*pb.ASTNode) (string, error) {
		parts := make([]string, len(kids))
		for i, k := range kids {
			s, err := renderNode(k, indent+2, g)
			if err != nil {
				return "", err
			}
			parts[i] = s
		}
		if len(parts) <= 1 {
			return strings.Join(parts, ""), nil
		}
		return pad + strings.Join(parts, pad+op+" "), nil
	}
	single := func(open, close string) (string, error) {
		kids := n.GetChildren()
		if len(kids) != 1 {
			return "", fmt.Errorf("%s: want 1 child, got %d", n.GetKind(), len(kids))
		}
		s, err := renderNode(kids[0], indent, g)
		if err != nil {
			return "", err
		}
		return open + " " + s + " " + close, nil
	}

	switch n.GetKind() {
	case compiler.KindNonterminal:
		return n.GetValue(), nil
	case compiler.KindTerminal:
		return g.termOpen + n.GetValue() + g.termClose, nil
	case compiler.KindScalar:
		// A scalar always lowers to a string field, so it is a text leaf; its
		// Value is the field name.
		return n.GetValue() + " " + g.comment("text"), nil
	case compiler.KindOptional:
		return single(g.optOpen, g.optClose)
	case compiler.KindRepetition:
		return single(g.repOpen, g.repClose)
	case compiler.KindGroup:
		return single(g.grpOpen, g.grpClose)
	case compiler.KindSequence:
		if len(n.GetChildren()) == 0 {
			return g.comment("empty"), nil
		}
		return joinWrapped(g.concat, n.GetChildren())
	case compiler.KindAlternation:
		return joinWrapped(g.alt, n.GetChildren())
	case compiler.KindRange:
		kids := n.GetChildren()
		if len(kids) != 2 {
			return "", fmt.Errorf("range: want 2 children, got %d", len(kids))
		}
		lo := g.termOpen + kids[0].GetValue() + g.termClose
		hi := g.termOpen + kids[1].GetValue() + g.termClose
		return lo + " " + g.comment("..") + " " + hi, nil
	default:
		return "", fmt.Errorf("unknown node kind %q", n.GetKind())
	}
}
