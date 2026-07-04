package metaparser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/accretional/gluon/lexkit"
	v1pb "github.com/accretional/gluon/pb"
	pb "github.com/accretional/gluon/v2/pb"
)

// CST parses a source document against a grammar and returns the
// concrete syntax tree. The request must carry both a GrammarDescriptor
// and a DocumentDescriptor; the TokenSequence (if present) is a hint for
// downstream tooling but is not required for parsing, since matching
// grammar terminals requires the source text itself.
func (s *Server) CST(ctx context.Context, req *pb.CstRequest) (*pb.ASTDescriptor, error) {
	ast, err := ParseCST(req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "CST: %v", err)
	}
	return ast, nil
}

// ParseCST is the pure-Go entry point behind the CST RPC. It converts
// the v2 grammar to v1's shape (by pretty-printing each rule's
// expressions back to EBNF text), delegates actual parsing to v1's
// grammar-driven parser, then converts the v1 AST back to the v2 shape.
//
// The start rule is the first rule in the grammar. If the grammar has
// no rules an error is returned.
func ParseCST(req *pb.CstRequest) (*pb.ASTDescriptor, error) {
	return ParseCSTWithOptions(req, nil)
}

// TokenMatchFunc matches a token starting at pos in src, returning the
// matched text and new position, or ("", -1) for no match. It is an alias
// of the lexkit type so callers depend only on v2/metaparser.
type TokenMatchFunc = lexkit.TokenMatchFunc

// ParseOptions carries language-specific lexical hooks for CST parsing.
// It mirrors lexkit.ASTParseOptions but lives on the v2 surface so callers
// need not import v1 lexkit directly. A nil *ParseOptions reproduces the
// default ParseCST behavior (standard EBNF-style parsing).
//
// TokenMatchers is the load-bearing field: it lets a grammar delegate a
// production to a custom Go scanner instead of grammar recursion — the
// intended hook for lexical constructs a CFG cannot express (e.g. XML
// names, character data, references, CDATA, comments, PIs). A referenced
// nonterminal with a registered matcher is matched by the matcher, which
// is consulted before grammar recursion.
type ParseOptions struct {
	// TokenMatchers maps production names to custom scanners.
	TokenMatchers map[string]TokenMatchFunc

	// CharClasses maps empty-bodied production names to single-rune tests.
	CharClasses map[string]func(rune) bool

	// IsLexical classifies a production as lexical (whitespace not skipped
	// inside it). If nil, names starting lowercase/'_' are lexical.
	IsLexical func(string) bool

	// Preprocessor transforms source text before parsing.
	Preprocessor func(string) string

	// StartRule selects the production to begin parsing from. Empty means
	// the grammar's first rule — the historical ParseCST behavior. Setting
	// it lets callers parse a fragment against any sub-rule of the same
	// grammar (e.g. one line/statement during recovery parsing) without
	// reordering the grammar's rules. The named rule must exist.
	StartRule string

	// DisableAutoComments turns off built-in //, /*, (* comment skipping
	// between tokens. Set true for languages (e.g. XML) where those byte
	// sequences are ordinary data.
	DisableAutoComments bool
}

func (o *ParseOptions) toLexkit() *lexkit.ASTParseOptions {
	if o == nil {
		return nil
	}
	return &lexkit.ASTParseOptions{
		TokenMatchers:       o.TokenMatchers,
		CharClasses:         o.CharClasses,
		IsLexical:           o.IsLexical,
		Preprocessor:        o.Preprocessor,
		DisableAutoComments: o.DisableAutoComments,
	}
}

// ParseCSTWithOptions is ParseCST with language-specific lexical hooks.
// Passing nil opts is identical to ParseCST. This entry point is purely
// additive: the CST RPC and existing ParseCST callers are unaffected.
func ParseCSTWithOptions(req *pb.CstRequest, opts *ParseOptions) (*pb.ASTDescriptor, error) {
	gd := req.GetGrammar()
	if gd == nil {
		return nil, errors.New("grammar is required")
	}
	doc := req.GetDocument()
	if doc == nil {
		return nil, errors.New("document is required")
	}
	if len(gd.GetRules()) == 0 {
		return nil, errors.New("grammar has no rules")
	}

	src := TextOf(doc)
	startRule := gd.GetRules()[0].GetName()
	if opts != nil && opts.StartRule != "" {
		found := false
		for _, r := range gd.GetRules() {
			if r.GetName() == opts.StartRule {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("start rule %q not in grammar", opts.StartRule)
		}
		startRule = opts.StartRule
	}

	v1gd := convertGrammarToV1(gd)
	v1ast, err := lexkit.ParseASTWithOptions(src, gd.GetName(), startRule, v1gd, opts.toLexkit())
	if err != nil {
		return nil, err
	}
	return convertASTToV2(v1ast, doc.GetUri()), nil
}

// convertGrammarToV1 turns a v2 GrammarDescriptor into a v1 one by
// serializing each rule's flat Production list back to ISO 14977 EBNF
// text. v1's parser will re-parse that text via its own EBNF lexer.
//
// The lex carried on the v1 result is the standard EBNF lex (so the
// rule-body re-parser can read concatenations / alternations) but
// its `whitespace` is REPLACED with whatever the v2 grammar's lex
// has — letting callers express "no internal whitespace" by handing
// in a v2 lex with no WHITESPACE delimiters. Default behavior
// (proto-ip's, proto-domain's) is unchanged when the v2 lex is the
// stock EBNFLexV2 with the four ASCII whitespace symbols.
func convertGrammarToV1(gd *pb.GrammarDescriptor) *v1pb.GrammarDescriptor {
	prods := make([]*v1pb.ProductionDescriptor, 0, len(gd.GetRules()))
	for _, rule := range gd.GetRules() {
		raw := printExpressions(rule.GetExpressions())
		prods = append(prods, &v1pb.ProductionDescriptor{
			Name:  rule.GetName(),
			Token: lexkit.RawToToken(raw),
		})
	}
	v1lex := lexkit.EBNFLex()
	v1lex.Whitespace = whitespaceFromV2Lex(gd.GetLex())
	return &v1pb.GrammarDescriptor{
		Lex:         v1lex,
		Productions: prods,
	}
}

// whitespaceFromV2Lex extracts the WHITESPACE delimiters from a v2
// LexDescriptor and returns them as v1 unicode.UTF8 messages. Each
// WHITESPACE-roled LexicalDelimiter contributes one rune (the first
// rune of its symbol field). Returns nil for a lex with no
// WHITESPACE entries — the parser will then skip nothing.
func whitespaceFromV2Lex(lex *pb.LexDescriptor) []*v1pb.UTF8 {
	var out []*v1pb.UTF8
	for _, sym := range lex.GetSymbols() {
		d := sym.GetDelimiter()
		if d == nil || d.GetKind() != pb.Delimiter_WHITESPACE {
			continue
		}
		s := d.GetSymbol()
		if s == "" {
			continue
		}
		for _, r := range s {
			out = append(out, lexkit.Char(r))
			break // one rune per delimiter symbol; matches EBNFLexV2
		}
	}
	return out
}

// printExpressions renders a flat Production list as EBNF source text
// that v1's parser can consume. Items are separated by a single space
// — v1's lexer skips whitespace between tokens.
func printExpressions(ps []*pb.Production) string {
	var b strings.Builder
	for i, p := range ps {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(printProduction(p))
	}
	return b.String()
}

// quoteTerminal renders a terminal as an EBNF literal for v1 to re-parse.
// EBNF terminals are literal — there are NO escape sequences (a backslash
// is a literal backslash) — so fmt's %q (Go quoting) is wrong: it would
// turn the terminal " into "\"" which v1 reads as the terminal \. Instead
// pick a quote style ("…", '…', or `…`) that does not occur in the value,
// matching what v1's lexer accepts. A value containing all three styles is
// not representable as an EBNF literal (rare; falls back to double quotes).
func quoteTerminal(s string) string {
	switch {
	case !strings.Contains(s, `"`):
		return `"` + s + `"`
	case !strings.Contains(s, "'"):
		return "'" + s + "'"
	case !strings.Contains(s, "`"):
		return "`" + s + "`"
	default:
		return `"` + s + `"`
	}
}

func printProduction(p *pb.Production) string {
	switch k := p.GetKind().(type) {
	case *pb.Production_Terminal:
		return quoteTerminal(k.Terminal)
	case *pb.Production_Nonterminal:
		return k.Nonterminal
	case *pb.Production_Delimiter:
		switch k.Delimiter {
		case pb.Delimiter_CONCATENATION:
			return ","
		case pb.Delimiter_ALTERNATION:
			return "|"
		case pb.Delimiter_WHITESPACE:
			return " "
		case pb.Delimiter_DEFINITION:
			return "="
		case pb.Delimiter_TERMINATION:
			return ";"
		}
		return ""
	case *pb.Production_Scoper:
		body := printExpressions(k.Scoper.GetBody())
		switch k.Scoper.GetKind() {
		case pb.Scoper_OPTIONAL:
			return "[ " + body + " ]"
		case pb.Scoper_REPETITION:
			return "{ " + body + " }"
		case pb.Scoper_GROUP:
			return "( " + body + " )"
		}
		return "( " + body + " )"
	case *pb.Production_Range:
		return fmt.Sprintf("%q ... %q", k.Range.GetLower(), k.Range.GetUpper())
	}
	return ""
}

// convertASTToV2 rewrites a v1 ASTDescriptor using v2's message shape.
// The only structural change is SourceLocation: v2's variant carries a
// URI and a length field; we populate the URI from the source document
// and leave length at zero since v1 tracks only a start offset.
func convertASTToV2(v1ast *v1pb.ASTDescriptor, docURI string) *pb.ASTDescriptor {
	if v1ast == nil {
		return nil
	}
	return &pb.ASTDescriptor{
		Language: v1ast.GetLanguage(),
		Version:  v1ast.GetVersion(),
		Root:     convertASTNode(v1ast.GetRoot(), docURI),
	}
}

func convertASTNode(n *v1pb.ASTNodeDescriptor, docURI string) *pb.ASTNode {
	if n == nil {
		return nil
	}
	kids := make([]*pb.ASTNode, 0, len(n.GetChildren()))
	for _, c := range n.GetChildren() {
		kids = append(kids, convertASTNode(c, docURI))
	}
	return &pb.ASTNode{
		Kind:     n.GetKind(),
		Value:    n.GetValue(),
		Children: kids,
		Location: convertLocation(n.GetLocation(), docURI),
	}
}

func convertLocation(loc *v1pb.SourceLocation, docURI string) *pb.SourceLocation {
	if loc == nil {
		return nil
	}
	return &pb.SourceLocation{
		Uri:    docURI,
		Offset: loc.GetOffset(),
		Line:   loc.GetLine(),
		Column: loc.GetColumn(),
	}
}
